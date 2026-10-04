package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

type ConnectivityChecker interface {
	CheckConnectivity(context.Context, Scope) error
}

type CacheChecker interface {
	CheckCache(context.Context, Scope) (CacheState, error)
}

type WorkspaceChecker interface {
	CheckWorkspace(context.Context, Scope) (WorkspaceState, error)
}

// Dependencies contains bounded doctor probes with explicit prerequisites.
type Dependencies struct {
	ConfigPath   func() string
	LookupEnv    func(string) (string, bool)
	Connectivity ConnectivityChecker
	Cache        CacheChecker
	Workspace    WorkspaceChecker
}

// Service runs every independent doctor probe.
type Service struct{ dependencies Dependencies }

// New creates a doctor service.
func New(dependencies Dependencies) *Service { return &Service{dependencies: dependencies} }

// Execute runs every bounded check without exposing dependency error text.
func (a *Service) Execute(ctx context.Context, input Input) (Output, error) {
	if pathLike(input.Environment) || pathLike(input.Workspace) {
		return Output{}, &errs.Error{ID: "doctor.run.usage", Kind: errs.KindUsage, Operation: "doctor.run", Summary: "Doctor scopes must be logical names, not paths.", Retryable: errs.Bool(false), CorrectiveAction: "Provide an environment alias or workspace name.", Validation: []errs.ValidationDetail{{Field: "scope", Code: "invalid", Message: "scope contains a path separator"}}}
	}
	scope := Scope{Environment: input.Environment, Workspace: input.Workspace}
	configuration := a.checkConfiguration(scope)
	checks := []Check{configuration}
	if configuration.Status != StatusPass {
		for _, id := range []string{"auth.pat.references", "auth.tableau.connectivity", "cache.status", "workspace.status"} {
			checks = append(checks, blocked(id, configuration.ID))
		}
	} else {
		pat := a.checkPAT(scope)
		checks = append(checks, pat)
		if pat.Status == StatusFail {
			checks = append(checks, blocked("auth.tableau.connectivity", pat.ID))
		} else {
			checks = append(checks, a.checkConnectivity(ctx, scope))
		}
		checks = append(checks, a.checkCache(ctx, scope), a.checkWorkspace(ctx, scope))
	}
	checks = append(checks, a.checkLogging())
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
	warningLabel := "warnings"
	if counts.Warn == 1 {
		warningLabel = "warning"
	}
	summary := fmt.Sprintf("%d checks completed: %d passed, %d %s, %d failed.", len(checks), counts.Pass, counts.Warn, warningLabel, counts.Fail)
	if counts.Blocked != 0 || counts.Info != 0 {
		summary = fmt.Sprintf("%d checks completed: %d passed, %d informational, %d %s, %d failed; %d blocked.", len(checks)-counts.Blocked, counts.Pass, counts.Info, counts.Warn, warningLabel, counts.Fail, counts.Blocked)
	}
	return Output{Status: status, Scope: scope, Counts: counts, Summary: summary, Checks: checks, Help: []string{commandhint.Target(scope.Environment, scope.Workspace, "doctor", "--full")}}, nil
}

func (a *Service) checkConfiguration(scope Scope) Check {
	const id = "config.valid"
	path := a.dependencies.ConfigPath()
	configuration, err := config.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return fail(id, "Configuration is missing.", "Create a TADX configuration with an environment profile.")
	}
	if err != nil {
		check := fail(id, "Configuration could not be validated.", "Review the selected CLI settings file, not the workspace manifest; correct the reported profile or field.")
		check.Cause, check.ConfigPath = err.Error(), path
		return check
	}
	if _, err := configuration.ResolveEnvironment(scope.Environment); err != nil {
		check := fail(id, "Configuration could not be validated.", "Review the selected CLI settings file, not the workspace manifest; correct the reported profile or field.")
		check.Cause, check.ConfigPath = err.Error(), path
		return check
	}
	return pass(id, "Configuration and environment resolution are valid.")
}

func (a *Service) checkPAT(scope Scope) (check Check) {
	const id = "auth.pat.references"
	configuration, err := config.Load(a.dependencies.ConfigPath())
	if err != nil {
		return fail(id, "PAT references could not be checked.", "Review the selected environment PAT references.")
	}
	environment, err := configuration.ResolveEnvironment(scope.Environment)
	if err != nil {
		return fail(id, "PAT references could not be checked.", "Review the selected environment PAT references.")
	}
	name, namePresent := a.dependencies.LookupEnv(environment.Auth.PATNameEnv)
	secret, secretPresent := a.dependencies.LookupEnv(environment.Auth.PATSecretEnv)
	state := PATState{
		ReferencesConfigured:    true,
		NameVariablePresent:     namePresent && name != "",
		SecretVariablePresent:   secretPresent && secret != "",
		StoredCredentialPresent: environment.Auth.CredentialRef != "",
		NameVariable:            environment.Auth.PATNameEnv,
		SecretVariable:          environment.Auth.PATSecretEnv,
	}
	switch {
	case state.NameVariablePresent && state.SecretVariablePresent:
		state.Source = "environment"
	case state.NameVariablePresent != state.SecretVariablePresent:
		state.Source = "incomplete_environment"
	case state.StoredCredentialPresent:
		state.Source = "credential_store_reference"
	default:
		state.Source = "unavailable"
	}
	defer func() {
		if state.NameVariable != "" || state.SecretVariable != "" || state.Source != "" {
			check.PAT = &state
		}
	}()
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

func (a *Service) checkConnectivity(ctx context.Context, scope Scope) Check {
	const id = "auth.tableau.connectivity"
	if err := a.dependencies.Connectivity.CheckConnectivity(ctx, scope); err != nil {
		return fail(id, "Tableau connectivity or PAT authentication failed.", "Verify the server, site, network, and PAT credentials.")
	}
	return pass(id, "Tableau connectivity and PAT authentication succeeded.")
}

func (a *Service) checkCache(ctx context.Context, scope Scope) Check {
	const id = "cache.status"
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

func (a *Service) checkWorkspace(ctx context.Context, scope Scope) Check {
	const id = "workspace.status"
	state, err := a.dependencies.Workspace.CheckWorkspace(ctx, scope)
	if err != nil {
		if scope.Workspace == "" {
			return warn(id, "Workspace status could not be resolved.", "Select or configure a logical workspace when artifact operations are needed.")
		}
		return fail(id, "The selected workspace could not be resolved.", "Verify the logical workspace name and registration.")
	}
	if !state.Available || !state.ManifestValid {
		return fail(id, "The selected workspace is unavailable or invalid.", "Repair the workspace registration and manifest.")
	}
	if state.DirtyArtifacts > 0 {
		return warn(id, "The selected workspace contains dirty artifacts.", "Review workspace status before overwriting or publishing artifacts.")
	}
	return pass(id, "The selected workspace is available and valid.")
}

func (a *Service) checkLogging() Check {
	const id = "logging.context"
	value, enabled := a.dependencies.LookupEnv("TADX_LOG_LEVEL")
	if enabled && strings.TrimSpace(value) == "" {
		return warn(id, "Logging context is invalid or unavailable.", "Correct TADX_LOG_LEVEL or unset it to use nonpersistent default logging.")
	}
	if enabled {
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
