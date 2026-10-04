package env

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Remover interface {
	Remove(context.Context, string) error
	PreviewRemove(context.Context, string) error
}

// Remove previews or removes one guarded environment profile.
func (s *Service) Remove(ctx context.Context, input RemoveInput) (RemoveOutput, error) {
	if s == nil || s.store == nil {
		return RemoveOutput{}, &errs.Error{ID: "env.profile.remove.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.remove", Summary: "Environment profile removal is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return RemoveOutput{}, &errs.Error{ID: "env.profile.remove.usage", Kind: errs.KindUsage, Operation: "env.profile.remove", Summary: "environment alias is required"}
	}
	remove := s.store.Remove
	if input.Preview {
		remove = s.store.PreviewRemove
	}
	if err := remove(ctx, input.Alias); err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias and default-environment guard, then retry.")
		id, summary := "env.profile.remove.write", "Environment profile could not be removed."
		if input.Preview {
			id, summary = "env.profile.remove.preview", "Environment profile preview failed."
		}
		return RemoveOutput{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.remove", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	if input.Preview {
		return RemoveOutput{Status: "preview", Environment: input.Alias, Help: []string{"Execution removes this profile after rechecking stored-credential and default-environment guards. The configuration has not been saved."}}, nil
	}
	return RemoveOutput{Status: "removed", Environment: input.Alias, Help: []string{"tadx env list"}}, nil
}

type RemoveInput struct {
	Preview bool   `json:"preview,omitempty"`
	Alias   string `json:"alias"`
}
type RemoveOutput struct {
	Status      string   `json:"status"`
	Environment string   `json:"environment"`
	Help        []string `json:"help"`
}
