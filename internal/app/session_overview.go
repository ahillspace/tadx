package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	sessionoverview "github.com/ahillspace/tadx/actions/session/overview"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type sessionOverviewReader struct{ runtime *runtimeDependencies }

func (r sessionOverviewReader) ReadOverview(ctx context.Context) (sessionoverview.State, error) {
	cfg, err := r.runtime.configuration()
	state := sessionoverview.State{Configuration: "configured", ReadSelection: "explicit_environment_required", WriteTarget: "explicit_environment_required", Workspace: sessionoverview.WorkspaceSelection{Status: "not_selected", Reason: "none"}}
	if errors.Is(err, os.ErrNotExist) {
		state.Configuration = "missing"
		cfg = config.Config{Version: config.CurrentVersion}
	} else if err != nil {
		return state, &errs.Error{ID: "session.overview.configuration", Kind: errs.KindOperation, Operation: "session.overview", Summary: "Local configuration could not be read or is invalid.", Cause: err, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: "Correct the reported field in the selected CLI settings file. No authentication was attempted."}
	}
	state.Mutations = value.MutationSetting{Scope: "site", Source: "site_selection_required"}
	selected, selectionErr := cfg.ResolveEnvironment("")
	if selectionErr == nil {
		state.Mutations, err = siteMutationSetting(cfg, selected)
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
	auth := newAuthStatus(r.runtime)
	for name, environment := range cfg.Environments {
		if err := ctx.Err(); err != nil {
			return state, err
		}
		status, err := auth.Execute(ctx, authstatus.Input{Environment: name})
		if err != nil {
			return state, err
		}
		credentials := "missing"
		if status.CredentialSource == "os_credential_store" {
			credentials = "stored_reference_unverified"
		}
		if status.CredentialSource == "environment" {
			credentials = "incomplete"
			if status.PATNamePresent && status.PATSecretPresent {
				credentials = "configured"
			}
		}
		environment.Alias = name
		mutations, err := siteMutationSetting(cfg, environment)
		if err != nil {
			return state, err
		}
		state.Environments = append(state.Environments, sessionoverview.Environment{Mutations: mutations, Name: name, Site: environment.SiteContentURL, ServerURL: environment.URL, CredentialSource: status.CredentialSource, Credentials: credentials, DefaultWorkspace: environment.DefaultWorkspace})
	}
	for name, workspace := range cfg.Workspaces {
		state.Workspaces = append(state.Workspaces, sessionoverview.Workspace{Name: name, Path: filepath.ToSlash(workspace.Path), Default: strings.EqualFold(name, cfg.DefaultWorkspace)})
	}
	record, workspaceErr := r.runtime.resolveWorkspace(ctx, cfg, "", selected.DefaultWorkspace)
	if ctx.Err() != nil {
		return state, ctx.Err()
	}
	if record.Name != "" {
		state.Workspace.Name = record.Name
		state.Workspace.Reason = record.SelectionReason
		state.Workspace.Status = "unavailable"
	}
	if workspaceErr == nil {
		state.Workspace.Status = "ready"
	}
	return state, nil
}
