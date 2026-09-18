package run

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
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

type CacheChecker interface {
	CheckCache(context.Context, Scope) (CacheState, error)
}

type WorkspaceChecker interface {
	CheckWorkspace(context.Context, Scope) (WorkspaceState, error)
}

type LoggingChecker interface {
	CheckLogging(context.Context, Scope) (LoggingState, error)
}

// Dependencies contains bounded doctor probes with explicit prerequisites.
type Dependencies struct {
	Configuration ConfigurationChecker
	PAT           PATChecker
	Connectivity  ConnectivityChecker
	Cache         CacheChecker
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
	configuration := a.checkConfiguration(ctx, scope)
	checks := []Check{configuration}
	if configuration.Status != StatusPass {
		for _, id := range []string{"auth.pat.references", "auth.tableau.connectivity", "cache.status", "workspace.status"} {
			checks = append(checks, blocked(id, configuration.ID))
		}
	} else {
		pat := a.checkPAT(ctx, scope)
		checks = append(checks, pat)
		if pat.Status == StatusFail {
			checks = append(checks, blocked("auth.tableau.connectivity", pat.ID))
		} else {
			checks = append(checks, a.checkConnectivity(ctx, scope))
		}
		checks = append(checks, a.checkCache(ctx, scope), a.checkWorkspace(ctx, scope))
	}
	checks = append(checks, a.checkLogging(ctx, scope))
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
		case StatusBlocked:
			counts.Blocked++
		case StatusInfo:
			counts.Info++
		}
	}
	summary := fmt.Sprintf("%d checks completed: %d passed, %d warnings, %d failed.", len(checks), counts.Pass, counts.Warn, counts.Fail)
	if counts.Blocked != 0 || counts.Info != 0 {
		summary = fmt.Sprintf("%d checks completed: %d passed, %d informational, %d warnings, %d failed; %d blocked.", len(checks)-counts.Blocked, counts.Pass, counts.Info, counts.Warn, counts.Fail, counts.Blocked)
	}
	return Output{Status: status, Scope: scope, Counts: counts, Summary: summary, Checks: checks, Help: []string{commandhint.Target(scope.Environment, scope.Workspace, "doctor", "--full")}}, nil
}

func (a *Action) checkConfiguration(ctx context.Context, scope Scope) Check {
	const id = "config.valid"
	if a.dependencies.Configuration == nil {
		return fail(id, "Configuration checking is unavailable.", "Configure the doctor configuration checker.")
	}
	state, err := a.dependencies.Configuration.CheckConfiguration(ctx, scope)
	if err != nil {
		check := fail(id, "Configuration could not be validated.", "Review the selected CLI settings file, not the workspace manifest; correct the reported profile or field.")
		check.Cause, check.ConfigPath = state.Cause, state.ConfigPath
		return check
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

func (a *Action) checkPAT(ctx context.Context, scope Scope) (check Check) {
	const id = "auth.pat.references"
	if a.dependencies.PAT == nil {
		return fail(id, "PAT reference checking is unavailable.", "Configure the PAT reference checker.")
	}
	state, err := a.dependencies.PAT.CheckPATReferences(ctx, scope)
	defer func() {
		if state.NameVariable != "" || state.SecretVariable != "" || state.Source != "" {
			check.PAT = &state
		}
	}()
	if err != nil {
		return fail(id, "PAT references could not be checked.", "Review the selected environment PAT references.")
	}
	if !state.ReferencesConfigured {
		return fail(id, "PAT environment-variable references are missing.", "Configure PAT name and secret environment-variable references.")
	}
	if state.NameVariablePresent != state.SecretVariablePresent {
		return fail(id, "One referenced PAT variable is absent.", "Set both referenced PAT variables or remove both so TADX can use the stored PAT.")
	}
	if state.NameVariablePresent && state.SecretVariablePresent {
		return pass(id, "PAT environment variables are present.")
	}
	if state.StoredCredentialPresent {
		return pass(id, "A stored PAT reference is configured; this local check does not verify its credentials.")
	}
	return fail(id, "No complete PAT source is configured.", "Run "+commandhint.Environment(scope.Environment, "auth", "login")+", or set both referenced PAT variables.")
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

func (a *Action) checkCache(ctx context.Context, scope Scope) Check {
	const id = "cache.status"
	if a.dependencies.Cache == nil {
		return fail(id, "Cache status checking is unavailable.", "Configure the local cache status checker.")
	}
	state, err := a.dependencies.Cache.CheckCache(ctx, scope)
	if err != nil {
		return fail(id, "Cache status could not be read.", "Repair or refresh the local cache.")
	}
	if !state.Present {
		return Check{ID: id, Status: StatusInfo, Summary: "No optional cache is present; live operations do not require it.", CorrectiveAction: "No action required for live operations. Refresh explicitly only when cached discovery is needed."}
	}
	if !state.Complete {
		return warn(id, "The current cache generation is incomplete.", "Run "+commandhint.Environment(scope.Environment, "cache", "refresh")+" and review any reported scope failures.")
	}
	if state.Stale {
		return warn(id, "The current cache generation is stale.", "Run "+commandhint.Environment(scope.Environment, "cache", "refresh")+" before relying on cached discovery.")
	}
	return pass(id, "The current cache generation is complete and fresh.")
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

func blocked(id, prerequisite string) Check {
	return Check{ID: id, Status: StatusBlocked, Summary: "Check did not run because its prerequisite failed.", BlockedBy: prerequisite, CorrectiveAction: "Resolve " + prerequisite + " before running this dependent check."}
}

func pathLike(value string) bool {
	return strings.ContainsAny(value, `/\\:`)
}
