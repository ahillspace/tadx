package catalog

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// TagWriter is the native value-based tag port shared by explicit catalog edits.
type TagWriter interface {
	AddTags(context.Context, value.LabelTarget, []string) ([]string, error)
	DeleteTag(context.Context, value.LabelTarget, string) error
}

type Change struct {
	Property string  `json:"property"`
	Before   *string `json:"before"`
	After    string  `json:"after"`
}

func validateTags(add, remove []string) string {
	if len(add)+len(remove) > 100 {
		return "at most 100 tag changes are allowed per item"
	}
	for _, tag := range add {
		if strings.TrimSpace(tag) == "" || utf8.RuneCountInString(tag) > 128 {
			return "added tags must contain 1 to 128 characters"
		}
	}
	for _, tag := range remove {
		if strings.TrimSpace(tag) == "" || tag != strings.TrimSpace(tag) || strings.ContainsAny(tag, "\x00\r\n") {
			return "removed tags must be exact nonempty selectors without surrounding whitespace or control characters"
		}
	}
	for _, tag := range add {
		if slices.Contains(remove, tag) {
			return "a tag cannot be both added and removed"
		}
	}
	return ""
}
func tagChanges(tags []string, observed bool, requestedAdd, requestedRemove []string) ([]Change, []string, []string) {
	changes := []Change{}
	add := []string{}
	remove := []string{}
	for _, tag := range requestedAdd {
		if !slices.Contains(add, tag) && (!observed || !slices.Contains(tags, tag)) {
			add = append(add, tag)
			changes = append(changes, Change{Property: "add_tag", After: tag})
		}
	}
	for _, tag := range requestedRemove {
		if !slices.Contains(remove, tag) && (!observed || slices.Contains(tags, tag)) {
			remove = append(remove, tag)
			changes = append(changes, Change{Property: "remove_tag", After: tag})
		}
	}
	return changes, add, remove
}

func verifyTagAcknowledgment(requested, acknowledged []string) error {
	for _, tag := range requested {
		if !slices.Contains(acknowledged, tag) {
			return &errs.Error{Kind: errs.KindOperation, Summary: "Tag response did not confirm every requested addition.", Phase: errs.PhaseVerification, Retryable: errs.Bool(false)}
		}
	}
	return nil
}

func propertyVerification(property string) error {
	return &errs.Error{Kind: errs.KindOperation, Summary: "Property response did not confirm requested " + property + ".", Retryable: errs.Bool(false), Phase: errs.PhaseVerification}
}
func retainedIdentity(previous, observed value.MetadataIdentity) value.MetadataIdentity {
	observed.MetadataID = cmp.Or(observed.MetadataID, previous.MetadataID)
	observed.LUID = cmp.Or(observed.LUID, previous.LUID)
	observed.Name = cmp.Or(observed.Name, previous.Name)
	observed.Type = cmp.Or(observed.Type, previous.Type)
	return observed
}

func failurePhase(cause error, outcome errs.Outcome) errs.Phase {
	phase := errs.PhaseSubmission
	structured, ok := errors.AsType[*errs.Error](cause)
	var verification interface{ VerificationFailed() bool }
	if (ok && structured.Phase == errs.PhaseVerification) || (errors.As(cause, &verification) && verification.VerificationFailed()) {
		phase = errs.PhaseVerification
	}
	if outcome == errs.OutcomeNotAttempted {
		phase = errs.PhaseValidation
	}
	return phase
}
