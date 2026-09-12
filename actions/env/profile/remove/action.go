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
	remove := a.remover.Remove
	if input.Preview {
		previewer, ok := a.remover.(interface {
			PreviewRemove(context.Context, string) error
		})
		if !ok {
			return Output{}, &errs.Error{ID: "env.profile.remove.preview", Kind: errs.KindRuntime, Operation: "env.profile.remove", Summary: "Profile preview is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a read-only profile preview store."}
		}
		remove = previewer.PreviewRemove
	}
	if err := remove(ctx, input.Alias); err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias and default-environment guard, then retry.")
		id, summary := "env.profile.remove.write", "Environment profile could not be removed."
		if input.Preview {
			id, summary = "env.profile.remove.preview", "Environment profile preview failed."
		}
		return Output{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.remove", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	if input.Preview {
		return Output{Status: "preview", Environment: input.Alias, Help: []string{"Execution removes this profile after rechecking stored-credential and default-environment guards. The configuration has not been saved."}}, nil
	}
	return Output{Status: "removed", Environment: input.Alias, Help: []string{"tadx env list"}}, nil
}
