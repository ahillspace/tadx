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
	setDefault := a.setter.SetDefault
	if input.Preview {
		previewer, ok := a.setter.(interface {
			PreviewSetDefault(context.Context, string) (bool, error)
		})
		if !ok {
			return Output{}, &errs.Error{ID: "env.profile.set-default.preview", Kind: errs.KindRuntime, Operation: "env.profile.set-default", Summary: "Profile preview is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a read-only profile preview store."}
		}
		setDefault = previewer.PreviewSetDefault
	}
	changed, err := setDefault(ctx, input.Alias)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		id, summary := "env.profile.set-default.write", "Default environment could not be updated."
		if input.Preview {
			id, summary = "env.profile.set-default.preview", "Environment profile preview failed."
		}
		return Output{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.set-default", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	status := "unchanged"
	if input.Preview {
		return Output{Status: "preview", DefaultEnvironment: input.Alias, WouldChange: &changed, Help: []string{"Execution selects this default environment after rechecking the alias. The configuration has not been saved."}}, nil
	}
	if changed {
		status = "updated"
	}
	return Output{Status: status, DefaultEnvironment: input.Alias, Help: []string{commandhint.Environment(input.Alias, "auth", "status")}}, nil
}
