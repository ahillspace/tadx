package status

import (
	"context"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Resolver interface {
	Resolve(context.Context, string) (Target, error)
}
type LookupEnv interface{ LookupEnv(string) (string, bool) }

type Action struct {
	resolver Resolver
	lookup   LookupEnv
}

func New(resolver Resolver, lookup LookupEnv) *Action {
	return &Action{resolver: resolver, lookup: lookup}
}

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil || a.lookup == nil {
		return Output{}, &errs.Error{ID: "auth.status.unconfigured", Kind: errs.KindRuntime, Operation: "auth.status", Summary: "Authentication status is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure environment and process-variable resolution before retrying."}
	}
	target, err := a.resolver.Resolve(ctx, input.Environment)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the selected environment configuration, then retry.")
		return Output{}, &errs.Error{ID: "auth.status.resolve", Kind: errs.KindOperation, Operation: "auth.status", Environment: input.Environment, Summary: "Authentication configuration could not be resolved.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	name, nameExists := a.lookup.LookupEnv(target.PATNameVariable)
	secret, secretExists := a.lookup.LookupEnv(target.PATSecretVariable)
	namePresent := nameExists && strings.TrimSpace(name) != ""
	secretPresent := secretExists && strings.TrimSpace(secret) != ""
	state := "incomplete"
	source := "none"
	if namePresent && secretPresent {
		state = "ready"
		source = "environment"
	} else if namePresent || secretPresent {
		source = "environment"
	} else if target.StoredCredentialReferencePresent {
		state = "ready"
		source = "os_credential_store"
	}
	return Output{Status: state, Environment: target.Environment, Default: target.Default, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, APIVersion: target.APIVersion, AuthType: target.AuthType, PATNameVariable: target.PATNameVariable, PATSecretVariable: target.PATSecretVariable, PATNamePresent: namePresent, PATSecretPresent: secretPresent, StoredCredentialReferencePresent: target.StoredCredentialReferencePresent, CredentialSource: source, DefaultWorkspace: target.DefaultWorkspace, Help: []string{commandhint.Environment(target.Environment, "auth", "check")}}, nil
}
