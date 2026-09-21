// Package overview presents local TADX setup without authenticating or creating state.
package overview

import (
	"context"
	"sort"

	"github.com/ahillspace/tadx/internal/value"
)

const (
	compactLimit = 10
	fullLimit    = 100
)

type Reader interface {
	ReadOverview(context.Context) (State, error)
}

// State contains only non-secret local configuration observations.
type State struct {
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
	state State
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

type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }

func (a *Action) Execute(ctx context.Context) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	state, err := a.reader.ReadOverview(ctx)
	if err != nil {
		return Output{}, err
	}
	state.Environments = append([]Environment(nil), state.Environments...)
	state.Workspaces = append([]Workspace(nil), state.Workspaces...)
	sort.Slice(state.Environments, func(i, j int) bool { return state.Environments[i].Name < state.Environments[j].Name })
	sort.Slice(state.Workspaces, func(i, j int) bool { return state.Workspaces[i].Name < state.Workspaces[j].Name })
	return Output{state: state}, nil
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

func result[E, W any](state State, environments []E, workspaces []W) Result[E, W] {
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
