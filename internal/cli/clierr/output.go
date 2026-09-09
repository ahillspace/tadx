package clierr

import "reflect"

type outputError struct {
	value any
	cause error
}

func (e outputError) Error() string        { return e.cause.Error() }
func (e outputError) Unwrap() error        { return e.cause }
func (e outputError) OperationOutput() any { return e.value }

// WithOutput carries known operation evidence through the error renderer.
// Presence never implies success; the action owns the outcome classification.
func WithOutput(value any, cause error) error {
	if cause == nil {
		return nil
	}
	if value == nil || reflect.ValueOf(value).IsZero() {
		return cause
	}
	return outputError{value: value, cause: cause}
}
