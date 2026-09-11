package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

type Reader interface {
	GetMetric(context.Context, string) (Metric, error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil {
		return Output{}, fail("pulse.metric.inspect.unconfigured", errs.KindRuntime, input, "Pulse metric retrieval is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if input.LUID == "" {
		return Output{}, fail("pulse.metric.inspect.usage", errs.KindUsage, input, "Pulse metric inspect requires an exact LUID.", nil)
	}
	metric, err := a.reader.GetMetric(ctx, input.LUID)
	if err != nil {
		var structured *errs.Error
		if input.Cache && errors.As(err, &structured) {
			return Output{}, err
		}
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact metric LUID and selected site, then retry.")
		return Output{}, &errs.Error{ID: "pulse.metric.inspect.failed", Kind: errs.KindOperation, Operation: "pulse.metric.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric retrieval failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if metric.LUID != input.LUID || metric.DefinitionLUID == "" || metric.Specification == nil {
		return Output{}, fail("pulse.metric.inspect.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched Pulse metric.", errors.New("metric requires matching LUID, definition LUID, and specification"))
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Metric: metric, RequestID: metric.RequestID, Help: []string{commandhint.Environment(input.Environment, "pulse", "metric", "fork", "--id", input.LUID, "--period", "LAST_30_DAYS", "--preview")}}, nil
}
func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact Pulse metric LUID and review the Tableau response."}
}
