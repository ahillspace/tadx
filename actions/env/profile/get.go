package profile

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type GetReader interface {
	Get(context.Context, string) (Profile, error)
}

type GetAction struct{ reader GetReader }

func NewGet(reader GetReader) *GetAction { return &GetAction{reader: reader} }

func (a *GetAction) Execute(ctx context.Context, input GetInput) (GetOutput, error) {
	if a == nil || a.reader == nil {
		return GetOutput{}, &errs.Error{ID: "env.profile.get.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.get", Summary: "Environment profile inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return GetOutput{}, &errs.Error{ID: "env.profile.get.usage", Kind: errs.KindUsage, Operation: "env.profile.get", Summary: "environment alias is required"}
	}
	profile, err := a.reader.Get(ctx, input.Alias)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		return GetOutput{}, &errs.Error{ID: "env.profile.get.read", Kind: errs.KindOperation, Operation: "env.profile.get", Environment: input.Alias, Summary: "Environment profile could not be read.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return GetOutput{Profile: profile, Help: []string{commandhint.Environment(profile.Alias, "auth", "status")}}, nil
}

type GetInput struct {
	Alias string `json:"alias"`
}

type GetOutput struct {
	Profile Profile  `json:"environment"`
	Help    []string `json:"help"`
}

type GetCompactProfile = Profile

type GetCompactResult struct {
	Profile GetCompactProfile `json:"environment"`
	Details string            `json:"details"`
	Help    []string          `json:"help"`
}

type GetFullResult struct {
	Profile Profile  `json:"environment"`
	Help    []string `json:"help"`
}

func (o GetOutput) CompactOutput() any {
	return GetCompactResult{Profile: o.Profile, Details: "--full", Help: o.Help}
}

func (o GetOutput) FullOutput() any { return GetFullResult{Profile: o.Profile, Help: o.Help} }
