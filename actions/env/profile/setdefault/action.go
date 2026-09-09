package setdefault

import (
	"context"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Setter interface {
	SetDefault(context.Context, string) (bool, error)
}
type Action struct{ setter Setter }

func New(setter Setter) *Action { return &Action{setter: setter} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.setter == nil {
		return Output{}, &errs.Error{ID: "env.profile.set-default.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.set-default", Summary: "Default environment selection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return Output{}, &errs.Error{ID: "env.profile.set-default.usage", Kind: errs.KindUsage, Operation: "env.profile.set-default", Summary: "environment alias is required"}
	}
	changed, err := a.setter.SetDefault(ctx, input.Alias)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return Output{}, &errs.Error{ID: "env.profile.set-default.write", Kind: errs.KindOperation, Operation: "env.profile.set-default", Environment: input.Alias, Summary: "Default environment could not be updated.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	status := "unchanged"
	if changed {
		status = "updated"
	}
	return Output{Status: status, DefaultEnvironment: input.Alias, Help: []string{commandhint.Environment(input.Alias, "auth", "status")}}, nil
}
