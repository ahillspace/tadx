package env

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type DefaultSetter interface {
	SetDefault(context.Context, string) (bool, error)
	PreviewSetDefault(context.Context, string) (bool, error)
}

// SetDefault previews or saves the default environment selection.
func (s *Service) SetDefault(ctx context.Context, input SetDefaultInput) (SetDefaultOutput, error) {
	if s == nil || s.store == nil {
		return SetDefaultOutput{}, &errs.Error{ID: "env.profile.set-default.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.set-default", Summary: "Default environment selection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if strings.TrimSpace(input.Alias) == "" {
		return SetDefaultOutput{}, &errs.Error{ID: "env.profile.set-default.usage", Kind: errs.KindUsage, Operation: "env.profile.set-default", Summary: "environment alias is required"}
	}
	setDefault := s.store.SetDefault
	if input.Preview {
		setDefault = s.store.PreviewSetDefault
	}
	changed, err := setDefault(ctx, input.Alias)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the exact environment alias, then retry.")
		id, summary := "env.profile.set-default.write", "Default environment could not be updated."
		if input.Preview {
			id, summary = "env.profile.set-default.preview", "Environment profile preview failed."
		}
		return SetDefaultOutput{}, &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "env.profile.set-default", Environment: input.Alias, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	status := "unchanged"
	if input.Preview {
		return SetDefaultOutput{Status: "preview", DefaultEnvironment: input.Alias, WouldChange: &changed, Help: []string{"Execution selects this default environment after rechecking the alias. The configuration has not been saved."}}, nil
	}
	if changed {
		status = "updated"
	}
	return SetDefaultOutput{Status: status, DefaultEnvironment: input.Alias, Help: []string{commandhint.Environment(input.Alias, "auth", "status")}}, nil
}

type SetDefaultInput struct {
	Preview bool   `json:"preview,omitempty"`
	Alias   string `json:"alias"`
}
type SetDefaultOutput struct {
	WouldChange        *bool    `json:"would_change,omitempty"`
	Status             string   `json:"status"`
	DefaultEnvironment string   `json:"default_environment"`
	Help               []string `json:"help"`
}
