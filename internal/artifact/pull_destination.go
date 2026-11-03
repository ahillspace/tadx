package artifact

import (
	"path/filepath"

	"github.com/ahillspace/tadx/internal/errs"
)

// requirePullDestination runs under the manager's workspace mutation lock,
// after existing-identity lookup and before staging or replacing an artifact.
func requirePullDestination(workspace, target, expected string) error {
	if expected == "" {
		return nil
	}
	relative, err := filepath.Rel(workspace, target)
	if err != nil {
		return err
	}
	if filepath.ToSlash(relative) != expected {
		return &errs.Error{ID: "artifact.destination_changed", Kind: errs.KindOperation,
			Summary: "Artifact destination changed after batch preparation.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted,
			Retryable: new(false), CorrectiveAction: "Review a new batch plan before pulling."}
	}
	return nil
}
