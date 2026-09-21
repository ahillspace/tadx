package get

import (
	"context"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Reader interface {
	Get(context.Context, string) (Profile, error)
}

type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, &errs.Error{ID: "env.profile.get.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.get", Summary: "Environment profile inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return Output{}, &errs.Error{ID: "env.profile.get.usage", Kind: errs.KindUsage, Operation: "env.profile.get", Summary: "environment alias is required"}
	}
	profile, err := a.reader.Get(ctx, input.Alias)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return Output{}, &errs.Error{ID: "env.profile.get.read", Kind: errs.KindOperation, Operation: "env.profile.get", Environment: input.Alias, Summary: "Environment profile could not be read.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return Output{Profile: profile, Help: []string{commandhint.Environment(profile.Alias, "auth", "status")}}, nil
}
