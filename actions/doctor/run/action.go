package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type ConfigurationChecker interface {
	CheckConfiguration(context.Context, Scope) (ConfigurationState, error)
}

type PATChecker interface {
	CheckPATReferences(context.Context, Scope) (PATState, error)
}

type ConnectivityChecker interface {
	CheckConnectivity(context.Context, Scope) (ConnectivityState, error)
}

type CatalogChecker interface {
	CheckCatalog(context.Context, Scope) (CatalogState, error)
}

type WorkspaceChecker interface {
	CheckWorkspace(context.Context, Scope) (WorkspaceState, error)
}

type LoggingChecker interface {
	CheckLogging(context.Context, Scope) (LoggingState, error)
}

// Dependencies contains independent bounded doctor probes.
type Dependencies struct {
	Configuration ConfigurationChecker
	PAT           PATChecker
	Connectivity  ConnectivityChecker
	Catalog       CatalogChecker
	Workspace     WorkspaceChecker
	Logging       LoggingChecker
}

// Action runs every independent doctor probe.
type Action struct{ dependencies Dependencies }

// New creates a doctor action.
func New(dependencies Dependencies) *Action { return &Action{dependencies: dependencies} }

// Execute runs every bounded check without exposing dependency error text.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil {
		return Output{}, &errs.Error{ID: "doctor.run.unconfigured", Kind: errs.KindRuntime, Operation: "doctor.run", Summary: "Doctor is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure doctor checks before retrying."}
	}
	if pathLike(input.Environment) || pathLike(input.Workspace) {
		return Output{}, &errs.Error{ID: "doctor.run.usage", Kind: errs.KindUsage, Operation: "doctor.run", Summary: "Doctor scopes must be logical names, not paths.", Retryable: errs.Bool(false), CorrectiveAction: "Provide an environment alias or workspace name.", Validation: []errs.ValidationDetail{{Field: "scope", Code: "invalid", Message: "scope contains a path separator"}}}
	}
	scope := Scope{Environment: input.Environment, Workspace: input.Workspace}
	checks := []Check{
		a.checkConfiguration(ctx, scope),
		a.checkPAT(ctx, scope),
		a.checkConnectivity(ctx, scope),
		a.checkCatalog(ctx, scope),
		a.checkWorkspace(ctx, scope),
		a.checkLogging(ctx, scope),
	}
	counts := Counts{}
	status := StatusPass
	for _, check := range checks {
		switch check.Status {
		case StatusPass:
			counts.Pass++
		case StatusWarn:
			counts.Warn++
			if status == StatusPass {
				status = StatusWarn
			}
		case StatusFail:
			counts.Fail++
			status = StatusFail
		}
	}
	summary := fmt.Sprintf("%d checks completed: %d passed, %d warnings, %d failed.", len(checks), counts.Pass, counts.Warn, counts.Fail)
	return Output{Status: status, Scope: scope, Counts: counts, Summary: summary, Checks: checks, Help: []string{"tadx doctor --full"}}, nil
}

func (a *Action) checkConfiguration(ctx context.Context, scope Scope) Check {
	const id = "config.valid"
	if a.dependencies.Configuration == nil {
		return fail(id, "Configuration checking is unavailable.", "Configure the doctor configuration checker.")
	}
	state, err := a.dependencies.Configuration.CheckConfiguration(ctx, scope)
	if err != nil {
		return fail(id, "Configuration could not be validated.", "Review the configuration and selected environment.")
	}
	if !state.Present {
		return fail(id, "Configuration is missing.", "Create a TADX configuration with an environment profile.")
	}
	if !state.Valid {
		return fail(id, "Configuration is invalid.", "Correct the configuration schema and environment profiles.")
	}
	if !state.EnvironmentResolved {
		return fail(id, "The selected environment cannot be resolved.", "Provide an existing environment alias or configure a default environment.")
	}
	return pass(id, "Configuration and environment resolution are valid.")
}

func (a *Action) checkPAT(ctx context.Context, scope Scope) Check {
	const id = "auth.pat.references"
	if a.dependencies.PAT == nil {
		return fail(id, "PAT reference checking is unavailable.", "Configure the PAT reference checker.")
	}
	state, err := a.dependencies.PAT.CheckPATReferences(ctx, scope)
	if err != nil {
		return fail(id, "PAT references could not be checked.", "Review the selected environment PAT references.")
	}
	if !state.ReferencesConfigured {
		return fail(id, "PAT environment-variable references are missing.", "Configure PAT name and secret environment-variable references.")
	}
	if !state.NameVariablePresent || !state.SecretVariablePresent {
		return fail(id, "One or more referenced PAT variables are absent.", "Set both referenced PAT variables without storing their values in TADX configuration.")
	}
	return pass(id, "PAT references and referenced variables are present.")
}

func (a *Action) checkConnectivity(ctx context.Context, scope Scope) Check {
	const id = "auth.tableau.connectivity"
	if a.dependencies.Connectivity == nil {
		return fail(id, "Tableau connectivity checking is unavailable.", "Configure the read-only Tableau connectivity checker.")
	}
	state, err := a.dependencies.Connectivity.CheckConnectivity(ctx, scope)
	if err != nil {
		return fail(id, "Tableau connectivity or PAT authentication failed.", "Verify the server, site, network, and PAT credentials.")
	}
	if !state.Reachable {
		return fail(id, "The configured Tableau endpoint is unreachable.", "Verify the server URL, network, proxy, and TLS configuration.")
	}
	if !state.Authenticated {
		return fail(id, "PAT authentication did not succeed.", "Verify PAT validity, expiration, permissions, and selected site.")
	}
	return pass(id, "Tableau connectivity and PAT authentication succeeded.")
}

func (a *Action) checkCatalog(ctx context.Context, scope Scope) Check {
	const id = "catalog.status"
	if a.dependencies.Catalog == nil {
		return fail(id, "Catalog status checking is unavailable.", "Configure the local catalog status checker.")
	}
	state, err := a.dependencies.Catalog.CheckCatalog(ctx, scope)
	if err != nil {
		return fail(id, "Catalog status could not be read.", "Repair or refresh the local catalog.")
	}
	if !state.Present {
		return warn(id, "No catalog generation is available.", "Run tadx catalog refresh for the selected environment.")
	}
	if !state.Complete {
		return warn(id, "The current catalog generation is incomplete.", "Run tadx catalog refresh and review any reported scope failures.")
	}
	if state.Stale {
		return warn(id, "The current catalog generation is stale.", "Run tadx catalog refresh before relying on cached discovery.")
	}
	return pass(id, "The current catalog generation is complete and fresh.")
}

func (a *Action) checkWorkspace(ctx context.Context, scope Scope) Check {
	const id = "workspace.status"
	if a.dependencies.Workspace == nil {
		return fail(id, "Workspace status checking is unavailable.", "Configure the logical workspace status checker.")
	}
	state, err := a.dependencies.Workspace.CheckWorkspace(ctx, scope)
	if err != nil {
		if scope.Workspace == "" {
			return warn(id, "Workspace status could not be resolved.", "Select or configure a logical workspace when artifact operations are needed.")
		}
		return fail(id, "The selected workspace could not be resolved.", "Verify the logical workspace name and registration.")
	}
	if !state.Selected {
		if scope.Workspace == "" {
			return warn(id, "No workspace is selected.", "Select or configure a logical workspace when artifact operations are needed.")
		}
		return fail(id, "The selected workspace does not exist.", "Verify the logical workspace name and registration.")
	}
	if !state.Available || !state.ManifestValid {
		return fail(id, "The selected workspace is unavailable or invalid.", "Repair the workspace registration and manifest.")
	}
	if state.DirtyArtifacts > 0 {
		return warn(id, "The selected workspace contains dirty artifacts.", "Review workspace status before overwriting or publishing artifacts.")
	}
	return pass(id, "The selected workspace is available and valid.")
}

func (a *Action) checkLogging(ctx context.Context, scope Scope) Check {
	const id = "logging.context"
	if a.dependencies.Logging == nil {
		return warn(id, "Logging context checking is unavailable.", "Inspect TADX_LOG_LEVEL when persistent diagnostic logging is required.")
	}
	state, err := a.dependencies.Logging.CheckLogging(ctx, scope)
	if err != nil || !state.Valid {
		return warn(id, "Logging context is invalid or unavailable.", "Correct TADX_LOG_LEVEL or unset it to use nonpersistent default logging.")
	}
	if state.Enabled {
		return pass(id, "Explicit logging context is configured.")
	}
	return pass(id, "Logging uses the nonpersistent default context.")
}

func pass(id, summary string) Check {
	return Check{ID: id, Status: StatusPass, Summary: summary, CorrectiveAction: "No action required."}
}

func warn(id, summary, correctiveAction string) Check {
	return Check{ID: id, Status: StatusWarn, Summary: summary, CorrectiveAction: correctiveAction}
}

func fail(id, summary, correctiveAction string) Check {
	return Check{ID: id, Status: StatusFail, Summary: summary, CorrectiveAction: correctiveAction}
}

func pathLike(value string) bool {
	return strings.ContainsAny(value, `/\\:`)
}
