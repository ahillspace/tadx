// Package errs defines TADX's stable structured error contract.
package errs

import (
	"errors"
	"fmt"
)

// Kind classifies an error without expanding the public exit-code space.
type Kind string

const (
	KindUsage     Kind = "usage"
	KindOperation Kind = "operation"
	KindRuntime   Kind = "runtime"
)

// ValidationDetail describes one invalid input field.
type ValidationDetail struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Error carries stable context for a failed TADX operation.
type Error struct {
	ID               string
	Kind             Kind
	Operation        string
	Selector         string
	Resource         string
	Environment      string
	Site             string
	Summary          string
	Cause            error
	Retryable        *bool
	CorrectiveAction string
	Validation       []ValidationDetail
	UpstreamStatus   int
	UpstreamCode     string
	UpstreamSummary  string
	UpstreamDetail   string
	TableauRequestID string
	TableauJobID     string
}

// New creates a structured error with a classification and summary.
func New(kind Kind, summary string) *Error {
	return &Error{Kind: kind, Summary: summary}
}

// Error implements error.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Summary != "" && e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Summary, e.Cause)
	}
	if e.Summary != "" {
		return e.Summary
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Kind)
}

// Unwrap exposes the upstream cause for errors.Is and errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Bool returns a pointer suitable for optional boolean fields.
func Bool(value bool) *bool { return &value }

// RetryAdvice returns retryability and corrective action carried by an error chain.
func RetryAdvice(err error) (*bool, string) {
	var carrier interface {
		Retryable() bool
		CorrectiveAction() string
	}
	if !errors.As(err, &carrier) {
		return nil, ""
	}
	return Bool(carrier.Retryable()), carrier.CorrectiveAction()
}

// TableauRequestID returns the first request ID carried by an error chain.
func TableauRequestID(err error) string {
	var structured *Error
	if errors.As(err, &structured) && structured.TableauRequestID != "" {
		return structured.TableauRequestID
	}
	var carrier interface{ RequestID() string }
	if errors.As(err, &carrier) {
		return carrier.RequestID()
	}
	return ""
}

// Payload is the stable serializable form of Error.
type Payload struct {
	ID               string             `json:"id,omitempty"`
	Kind             Kind               `json:"kind"`
	Operation        string             `json:"operation,omitempty"`
	Selector         string             `json:"selector,omitempty"`
	Resource         string             `json:"resource,omitempty"`
	Environment      string             `json:"environment,omitempty"`
	Site             string             `json:"site,omitempty"`
	Summary          string             `json:"summary"`
	UpstreamCause    string             `json:"upstream_cause,omitempty"`
	Retryable        *bool              `json:"retryable,omitempty"`
	CorrectiveAction string             `json:"corrective_action,omitempty"`
	Validation       []ValidationDetail `json:"validation,omitempty"`
	UpstreamStatus   int                `json:"upstream_status,omitempty"`
	UpstreamCode     string             `json:"upstream_code,omitempty"`
	UpstreamSummary  string             `json:"upstream_summary,omitempty"`
	UpstreamDetail   string             `json:"upstream_detail,omitempty"`
	TableauRequestID string             `json:"tableau_request_id,omitempty"`
	TableauJobID     string             `json:"tableau_job_id,omitempty"`
}

// Envelope is the top-level structured error document.
type Envelope struct {
	Error Payload `json:"error"`
}

// Structure converts any error to the stable error envelope.
func Structure(err error) Envelope {
	var structured *Error
	if errors.As(err, &structured) {
		kind := structured.Kind
		if kind == "" {
			kind = KindRuntime
		}
		payload := Payload{
			ID:               structured.ID,
			Kind:             kind,
			Operation:        structured.Operation,
			Selector:         structured.Selector,
			Resource:         structured.Resource,
			Environment:      structured.Environment,
			Site:             structured.Site,
			Summary:          structured.Summary,
			Retryable:        structured.Retryable,
			CorrectiveAction: structured.CorrectiveAction,
			Validation:       structured.Validation,
			UpstreamStatus:   structured.UpstreamStatus,
			UpstreamCode:     structured.UpstreamCode,
			UpstreamSummary:  structured.UpstreamSummary,
			UpstreamDetail:   structured.UpstreamDetail,
			TableauRequestID: structured.TableauRequestID,
			TableauJobID:     structured.TableauJobID,
		}
		if payload.Summary == "" {
			payload.Summary = structured.Error()
		}
		if structured.Cause != nil {
			payload.UpstreamCause = structured.Cause.Error()
			retryable, correctiveAction := RetryAdvice(structured.Cause)
			if payload.Retryable == nil {
				payload.Retryable = retryable
			}
			if payload.CorrectiveAction == "" {
				payload.CorrectiveAction = correctiveAction
			}
			status, code, summary, detail := tableauUpstream(structured.Cause)
			if payload.UpstreamStatus == 0 {
				payload.UpstreamStatus = status
			}
			if payload.UpstreamCode == "" {
				payload.UpstreamCode = code
			}
			if payload.UpstreamSummary == "" {
				payload.UpstreamSummary = summary
			}
			if payload.UpstreamDetail == "" {
				payload.UpstreamDetail = detail
			}
			if payload.TableauRequestID == "" {
				payload.TableauRequestID = TableauRequestID(structured.Cause)
			}
		}
		return Envelope{Error: payload}
	}
	message := "unknown error"
	if err != nil {
		message = err.Error()
	}
	return Envelope{Error: Payload{Kind: KindRuntime, Summary: message}}
}

func tableauUpstream(err error) (int, string, string, string) {
	var carrier interface {
		HTTPStatus() int
		TableauCode() string
		TableauSummary() string
		TableauDetail() string
	}
	if !errors.As(err, &carrier) {
		return 0, "", "", ""
	}
	return carrier.HTTPStatus(), carrier.TableauCode(), carrier.TableauSummary(), carrier.TableauDetail()
}

// ExitCode maps errors to the AXI exit-code contract.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var structured *Error
	if errors.As(err, &structured) && structured.Kind == KindUsage {
		return 2
	}
	return 1
}
