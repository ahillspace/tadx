package artifact

import (
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

// MapResolutionError adds bounded, contextual recovery evidence to an ambiguous selector.
// All other errors retain their original identity.
func MapResolutionError(operation, workspace, resource string, cause error) error {
	ambiguous, ok := errors.AsType[*AmbiguousSelectorError](cause)
	if !ok {
		return cause
	}
	ambiguous.FullStatusCommand = commandhint.Command("workspace", "status", "--workspace", workspace, "--full")
	structured := &errs.Error{
		ID:               operation + ".ambiguous",
		Kind:             errs.KindUsage,
		Operation:        operation,
		Resource:         resource,
		Summary:          "The managed artifact selector matched more than one exact artifact.",
		Cause:            cause,
		Retryable:        errs.Bool(false),
		CorrectiveAction: "Review the bounded candidates with " + ambiguous.FullStatusCommand + ", then select one exact workspace-relative artifact path.",
		Phase:            errs.PhaseValidation,
		Outcome:          errs.OutcomeNotAttempted,
	}
	return ambiguityOutputError{cause: structured, output: AmbiguousOutput{
		Status:            "ambiguous",
		Workspace:         workspace,
		Kind:              ambiguous.Kind,
		Name:              ambiguous.Name,
		LUID:              ambiguous.LUID,
		Candidates:        append([]AmbiguousCandidate(nil), ambiguous.Candidates...),
		Truncated:         ambiguous.Truncated,
		FullStatusCommand: ambiguous.FullStatusCommand,
	}}
}

// AmbiguousOutput is the bounded candidate evidence carried with a failed selection.
type AmbiguousOutput struct {
	Status            string               `json:"status"`
	Workspace         string               `json:"workspace"`
	Kind              string               `json:"kind"`
	Name              string               `json:"name,omitempty"`
	LUID              string               `json:"luid,omitempty"`
	Candidates        []AmbiguousCandidate `json:"candidates"`
	Truncated         int                  `json:"truncated"`
	FullStatusCommand string               `json:"full_status_command"`
}

type ambiguityOutputError struct {
	cause  error
	output AmbiguousOutput
}

func (e ambiguityOutputError) Error() string        { return e.cause.Error() }
func (e ambiguityOutputError) Unwrap() error        { return e.cause }
func (e ambiguityOutputError) OperationOutput() any { return e.output }
