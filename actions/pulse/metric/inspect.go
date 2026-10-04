package metric

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
	"github.com/ahillspace/tadx/internal/readsource"
)

type InspectReader interface {
	GetMetric(context.Context, string) (Metric, error)
}

func inspectValidated(ctx context.Context, reader InspectReader, input InspectInput) (InspectOutput, error) {
	input.LUID = strings.TrimSpace(input.LUID)
	metric, err := reader.GetMetric(ctx, input.LUID)
	if err != nil {
		var structured *errs.Error
		if input.Cache && errors.As(err, &structured) {
			return InspectOutput{}, err
		}
		if inspectIsNotFound(err) {
			return InspectOutput{}, &errs.Error{ID: "pulse.metric.inspect.not_found", Kind: errs.KindOperation, Operation: "pulse.metric.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: fmt.Sprintf("Pulse metric %q was not found or is inaccessible in the selected environment.", input.LUID), Cause: err, Retryable: errs.Bool(false), CorrectiveAction: fmt.Sprintf("Verify exact metric LUID %q in environment %q and site %q, then reconcile that resource. Do not repair CLI configuration for a resource-level 404.", input.LUID, input.Environment, input.Site), TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted}
		}
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact metric LUID and selected site, then retry.")
		return InspectOutput{}, &errs.Error{ID: "pulse.metric.inspect.failed", Kind: errs.KindOperation, Operation: "pulse.metric.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric retrieval failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if metric.LUID != input.LUID || metric.DefinitionLUID == "" || metric.Specification == nil {
		return InspectOutput{}, inspectFail("pulse.metric.inspect.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched Pulse metric.", errors.New("metric requires matching LUID, definition LUID, and specification"))
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Metric: metric, RequestID: metric.RequestID, Help: []string{commandhint.Environment(input.Environment, "pulse", "metric", "fork", "--id", input.LUID, "--period", "LAST_30_DAYS", "--preview")}}, nil
}

func inspectIsNotFound(err error) bool {
	var status interface{ HTTPStatus() int }
	return errors.As(err, &status) && status.HTTPStatus() == 404
}

func inspectFail(id string, kind errs.Kind, input InspectInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact Pulse metric LUID and review the Tableau response."}
}

type InspectInput struct {
	Environment string
	Site        string
	LUID        string
	Cache       bool
}
type InspectOutput struct {
	Status      string
	Environment string
	Site        string
	Metric      Metric
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}
type InspectCompactMetric struct {
	LUID           string `json:"luid"`
	Name           string `json:"name,omitempty"`
	DefinitionLUID string `json:"definition_luid"`
	IsDefault      bool   `json:"is_default"`
}
type InspectCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Metric      InspectCompactMetric `json:"metric"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Metric      Metric               `json:"metric"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o InspectOutput) CompactOutput() any {
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Metric: InspectCompactMetric{LUID: o.Metric.LUID, Name: o.Metric.Name, DefinitionLUID: o.Metric.DefinitionLUID, IsDefault: o.Metric.IsDefault}, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o InspectOutput) FullOutput() any {
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Metric: o.Metric, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func inspectValidateInput(input InspectInput) error {
	if strings.TrimSpace(input.LUID) == "" {
		return inspectFail("pulse.metric.inspect.usage", errs.KindUsage, input, "Pulse metric inspect requires an exact LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.LUID); err != nil {
		return inspectFail("pulse.metric.inspect.usage", errs.KindUsage, input, "Pulse metric inspect requires a well-formed exact LUID.", err)
	}
	return nil
}
