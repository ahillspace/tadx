// Package session presents local TADX setup without authenticating or creating state.
package session

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

const (
	compactLimit = 10
	fullLimit    = 100
)

// WorkspaceResolution contains only the non-secret local selection facts.
type WorkspaceResolution struct {
	Name, Reason string
}

// Dependencies supplies local mechanisms while the service owns aggregation.
type Dependencies struct {
	ReadConfiguration func() (config.Config, error)
	SiteSetting       func(config.Config, config.Environment) (value.MutationSetting, error)
	LookupEnv         coreauth.LookupEnv
	ResolveWorkspace  func(context.Context, config.Config, string, string) (WorkspaceResolution, error)
}

// overviewState contains only non-secret local configuration observations.
type overviewState struct {
	Configuration   string
	ReadEnvironment string
	ReadSelection   string
	WriteTarget     string
	Environments    []Environment
	Workspace       WorkspaceSelection
	Workspaces      []Workspace
	Mutations       value.MutationSetting
}

type Environment struct {
	Mutations        value.MutationSetting `json:"mutations"`
	Name             string                `json:"name"`
	Site             string                `json:"site"`
	CredentialSource string                `json:"credential_source"`
	Credentials      string                `json:"credentials"`
	ServerURL        string                `json:"server_url"`
	DefaultWorkspace string                `json:"default_workspace,omitempty"`
}

type Workspace struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Path    string `json:"path"`
}

type WorkspaceSelection struct {
	Name   string `json:"name,omitempty"`
	Reason string `json:"selection_reason"`
	Status string `json:"status"`
}

type Page[T any] struct {
	Total    int  `json:"total"`
	Returned int  `json:"returned"`
	More     bool `json:"more"`
	Items    []T  `json:"items"`
}

type Output struct {
	state overviewState
}

type Result[E, W any] struct {
	Status           string                `json:"status"`
	Configuration    string                `json:"configuration"`
	ReadEnvironment  string                `json:"read_environment,omitempty"`
	ReadSelection    string                `json:"read_selection"`
	WriteTarget      string                `json:"write_target"`
	AuthVerification string                `json:"auth_verification"`
	Environments     Page[E]               `json:"environments"`
	Workspace        WorkspaceSelection    `json:"workspace"`
	Workspaces       Page[W]               `json:"workspaces"`
	Mutations        value.MutationSetting `json:"mutations"`
	Help             []string              `json:"help"`
}

type CompactEnvironment struct {
	MutationsEnabled bool   `json:"mutations_enabled"`
	Name             string `json:"name"`
	Site             string `json:"site"`
	CredentialSource string `json:"credential_source"`
	Credentials      string `json:"credentials"`
}

type CompactWorkspace struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
}

type Service struct{ dependencies Dependencies }

func New(dependencies Dependencies) *Service { return &Service{dependencies: dependencies} }

func (s *Service) Execute(ctx context.Context) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	state, err := s.readOverview(ctx)
	if err != nil {
		return Output{}, err
	}
	slices.SortFunc(state.Environments, func(left, right Environment) int { return cmp.Compare(left.Name, right.Name) })
	slices.SortFunc(state.Workspaces, func(left, right Workspace) int { return cmp.Compare(left.Name, right.Name) })
	return Output{state: state}, nil
}

func (s *Service) readOverview(ctx context.Context) (overviewState, error) {
	cfg, err := s.dependencies.ReadConfiguration()
	state := overviewState{Configuration: "configured", ReadSelection: "explicit_environment_required", WriteTarget: "explicit_environment_required", Workspace: WorkspaceSelection{Status: "not_selected", Reason: "none"}}
	if errors.Is(err, os.ErrNotExist) {
		state.Configuration = "missing"
		cfg = config.Config{Version: config.CurrentVersion}
	} else if err != nil {
		return state, &errs.Error{ID: "session.overview.configuration", Kind: errs.KindOperation, Operation: "session.overview", Summary: "Local configuration could not be read or is invalid.", Cause: err, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: "Correct the reported field in the selected CLI settings file. No authentication was attempted."}
	}
	state.Mutations = value.MutationSetting{Scope: "site", Source: "site_selection_required"}
	selected, selectionErr := cfg.ResolveEnvironment("")
	if selectionErr == nil {
		state.Mutations, err = s.dependencies.SiteSetting(cfg, selected)
		if err != nil {
			return state, err
		}
		state.ReadEnvironment = selected.Alias
		state.ReadSelection = "configured_default"
		if cfg.DefaultEnvironment == "" {
			state.ReadSelection = "only_environment"
		}
	}
	if len(cfg.Environments) == 1 {
		state.WriteTarget = "only_environment"
	}
	if len(cfg.Environments) == 0 {
		state.WriteTarget = "environment_setup_required"
		state.ReadSelection = "environment_setup_required"
	}
	for name, environment := range cfg.Environments {
		if err := ctx.Err(); err != nil {
			return state, err
		}
		resolved, err := cfg.ResolveEnvironment(name)
		if err != nil {
			return state, err
		}
		readiness := coreauth.InspectLocalPATReadiness(resolved.Auth.PATNameEnv, resolved.Auth.PATSecretEnv, resolved.Auth.CredentialRef != "", s.dependencies.LookupEnv)
		credentials := "missing"
		if readiness.Source == "os_credential_store" {
			credentials = "stored_reference_unverified"
		}
		if readiness.Source == "environment" {
			credentials = "incomplete"
			if readiness.NamePresent && readiness.SecretPresent {
				credentials = "configured"
			}
		}
		environment.Alias = name
		mutations, err := s.dependencies.SiteSetting(cfg, environment)
		if err != nil {
			return state, err
		}
		state.Environments = append(state.Environments, Environment{Mutations: mutations, Name: name, Site: environment.SiteContentURL, ServerURL: environment.URL, CredentialSource: readiness.Source, Credentials: credentials, DefaultWorkspace: environment.DefaultWorkspace})
	}
	for name, workspace := range cfg.Workspaces {
		state.Workspaces = append(state.Workspaces, Workspace{Name: name, Path: filepath.ToSlash(workspace.Path), Default: strings.EqualFold(name, cfg.DefaultWorkspace)})
	}
	record, workspaceErr := s.dependencies.ResolveWorkspace(ctx, cfg, "", selected.DefaultWorkspace)
	if ctx.Err() != nil {
		return state, ctx.Err()
	}
	if record.Name != "" {
		state.Workspace.Name = record.Name
		state.Workspace.Reason = record.Reason
		state.Workspace.Status = "unavailable"
	}
	if workspaceErr == nil {
		state.Workspace.Status = "ready"
	}
	return state, nil
}

func (o Output) CompactOutput() any {
	environments := make([]CompactEnvironment, 0, min(compactLimit, len(o.state.Environments)))
	for _, e := range o.state.Environments[:min(compactLimit, len(o.state.Environments))] {
		environments = append(environments, CompactEnvironment{e.Mutations.Enabled, e.Name, e.Site, e.CredentialSource, e.Credentials})
	}
	workspaces := make([]CompactWorkspace, 0, min(compactLimit, len(o.state.Workspaces)))
	for _, w := range o.state.Workspaces[:min(compactLimit, len(o.state.Workspaces))] {
		workspaces = append(workspaces, CompactWorkspace{w.Name, w.Default})
	}
	return result(o.state, environments, workspaces)
}

func (o Output) FullOutput() any {
	environments := append([]Environment{}, o.state.Environments[:min(fullLimit, len(o.state.Environments))]...)
	workspaces := append([]Workspace{}, o.state.Workspaces[:min(fullLimit, len(o.state.Workspaces))]...)
	return result(o.state, environments, workspaces)
}

func result[E, W any](state overviewState, environments []E, workspaces []W) Result[E, W] {
	help := []string{"tadx --help"}
	if state.Configuration == "missing" || len(state.Environments) == 0 {
		help = append(help, "tadx env add --help")
	}
	if len(state.Environments) > len(environments) {
		help = append(help, "tadx env list")
	}
	if len(state.Workspaces) > len(workspaces) {
		help = append(help, "tadx workspace list --limit 1000")
	}
	if len(state.Workspaces) == 0 {
		help = append(help, "tadx workspace create --help")
	} else if state.Workspace.Status != "ready" && len(state.Workspaces) <= len(workspaces) {
		help = append(help, "tadx workspace list")
	}
	return Result[E, W]{
		Status: "local_overview", Configuration: state.Configuration, ReadEnvironment: state.ReadEnvironment,
		ReadSelection: state.ReadSelection, WriteTarget: state.WriteTarget, AuthVerification: "not_checked",
		Environments: Page[E]{Total: len(state.Environments), Returned: len(environments), More: len(state.Environments) > len(environments), Items: environments},
		Workspace:    state.Workspace,
		Workspaces:   Page[W]{Total: len(state.Workspaces), Returned: len(workspaces), More: len(state.Workspaces) > len(workspaces), Items: workspaces},
		Mutations:    state.Mutations, Help: help,
	}
}
