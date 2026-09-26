package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Reader is the action-owned exact definition seam.
type Reader interface {
	GetDefinition(context.Context, string) (Definition, error)
}

// Action inspects one exact Pulse definition.
type Action struct{ reader Reader }

// New creates a Pulse definition inspect action.
func New(reader Reader) *Action { return &Action{reader: reader} }

// Execute inspects and verifies one exact definition identity.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil {
		return Output{}, definitionError("pulse.definition.inspect.unconfigured", errs.KindRuntime, input, "Pulse definition retrieval is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	definition, err := a.reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		var structured *errs.Error
		if input.Cache && errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact definition LUID and selected Tableau site, then retry.")
		return Output{}, &errs.Error{ID: "pulse.definition.inspect.failed", Kind: errs.KindOperation, Operation: "pulse.definition.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse definition retrieval failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if definition.LUID != input.LUID {
		return Output{}, definitionError("pulse.definition.inspect.invalid_response", errs.KindOperation, input, "Tableau returned a different Pulse definition.", errors.New("definition LUID did not match the requested LUID"))
	}
	if definition.Name == "" || definition.DatasourceLUID == "" {
		return Output{}, definitionError("pulse.definition.inspect.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete Pulse definition.", errors.New("definition requires name and datasource LUID"))
	}
	requestID := definition.RequestID
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Definition: definition, RequestID: requestID, Help: []string{commandhint.Environment(input.Environment, "pulse", "definition", "pull", "--id", input.LUID)}}, nil
}

func definitionError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact Pulse definition LUID and review the Tableau response."}
}
