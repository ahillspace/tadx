package policy

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type SamplesOutput struct {
	Status       string   `json:"status"`
	Files        []string `json:"files"`
	SystemPath   string   `json:"system_path"`
	Instructions []string `json:"instructions"`
}

// ValidateSamplesDirectory rejects a missing path without changing its spelling.
func ValidateSamplesDirectory(directory string) error {
	if strings.TrimSpace(directory) == "" {
		return &errs.Error{ID: "policy.samples.usage", Kind: errs.KindUsage, Operation: "policy.samples", Summary: "--output is required.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	}
	return nil
}
