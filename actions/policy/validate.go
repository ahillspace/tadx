package policy

import (
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type ValidationOutput struct {
	Status              string   `json:"status"`
	Candidate           string   `json:"candidate"`
	AllowedCapabilities int      `json:"allowed_capabilities"`
	RemoteMutations     bool     `json:"remote_mutations"`
	ProtectionChecked   bool     `json:"protection_checked"`
	Help                []string `json:"help"`
}

// ValidateCandidatePath rejects a missing candidate without changing its spelling.
func ValidateCandidatePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return &errs.Error{ID: "policy.validate.usage", Kind: errs.KindUsage, Operation: "policy.validate", Summary: "An exact candidate file is required.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	}
	return nil
}
