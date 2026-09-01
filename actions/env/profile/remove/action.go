package remove

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Remover interface {
	Remove(context.Context, string) error
}
type Action struct{ remover Remover }

func New(remover Remover) *Action { return &Action{remover: remover} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.remover == nil {
		return Output{}, &errs.Error{ID: "env.profile.remove.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.remove", Summary: "Environment profile removal is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return Output{}, &errs.Error{ID: "env.profile.remove.usage", Kind: errs.KindUsage, Operation: "env.profile.remove", Summary: "environment alias is required"}
	}
	if err := a.remover.Remove(ctx, input.Alias); err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias and default-environment guard, then retry.")
		return Output{}, &errs.Error{ID: "env.profile.remove.write", Kind: errs.KindOperation, Operation: "env.profile.remove", Environment: input.Alias, Summary: "Environment profile could not be removed.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	return Output{Status: "removed", Environment: input.Alias, Help: []string{"tadx env list"}}, nil
}
