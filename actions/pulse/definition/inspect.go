package definition

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// Reader is the action-owned exact definition seam.
type InspectReader interface {
	GetDefinition(context.Context, string) (Definition, error)
}

// Inspect inspects and verifies one exact definition identity.
func Inspect(ctx context.Context, reader InspectReader, input InspectInput) (InspectOutput, error) {
	input.LUID = strings.TrimSpace(input.LUID)
	definition, err := reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		var structured *errs.Error
		if input.Cache && errors.As(err, &structured) {
			return InspectOutput{}, err
		}
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact definition LUID and selected Tableau site, then retry.")
		return InspectOutput{}, &errs.Error{ID: "pulse.definition.inspect.failed", Kind: errs.KindOperation, Operation: "pulse.definition.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse definition retrieval failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if definition.LUID != input.LUID {
		return InspectOutput{}, inspectDefinitionError("pulse.definition.inspect.invalid_response", errs.KindOperation, input, "Tableau returned a different Pulse definition.", errors.New("definition LUID did not match the requested LUID"))
	}
	if definition.Name == "" || definition.DatasourceLUID == "" {
		return InspectOutput{}, inspectDefinitionError("pulse.definition.inspect.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete Pulse definition.", errors.New("definition requires name and datasource LUID"))
	}
	requestID := definition.RequestID
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Definition: definition, RequestID: requestID, Help: []string{commandhint.Environment(input.Environment, "pulse", "definition", "pull", "--id", input.LUID)}}, nil
}

func inspectDefinitionError(id string, kind errs.Kind, input InspectInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.inspect", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact Pulse definition LUID and review the Tableau response."}
}

// Input selects one authoritative Pulse definition.
type InspectInput struct {
	Environment string
	Site        string
	LUID        string
	Cache       bool
}

// Output retains complete details before projection.
type InspectOutput struct {
	Status      string
	Environment string
	Site        string
	Definition  Definition
	RequestID   string
	Help        []string
	Source      *readsource.Metadata
}

// CompactDefinition identifies the exact Pulse definition.
type InspectCompactDefinition struct {
	LUID                 string   `json:"luid"`
	Name                 string   `json:"name"`
	DatasourceLUID       string   `json:"datasource_luid"`
	MeasureField         string   `json:"measure_field"`
	Aggregation          string   `json:"aggregation"`
	TimeDimension        string   `json:"time_dimension"`
	RunningTotal         bool     `json:"running_total"`
	Temporality          string   `json:"temporality,omitempty"`
	AllowedDimensions    []string `json:"allowed_dimensions,omitempty"`
	DimensionsOmitted    int      `json:"dimensions_omitted,omitempty"`
	AllowedGranularities []string `json:"allowed_granularities,omitempty"`
	GranularitiesOmitted int      `json:"granularities_omitted,omitempty"`
}

// CompactResult is the default projection.
type InspectCompactResult struct {
	Status      string                   `json:"status"`
	Environment string                   `json:"environment,omitempty"`
	Site        string                   `json:"site,omitempty"`
	Definition  InspectCompactDefinition `json:"definition"`
	Details     string                   `json:"details"`
	Help        []string                 `json:"help"`
	Source      *readsource.Metadata     `json:"source,omitempty"`
}

// FullResult is the expanded bounded projection.
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Definition  Definition           `json:"definition"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

// CompactOutput returns stable identity fields.
func (o InspectOutput) CompactOutput() any {
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Definition: inspectCompactDefinition(o.Definition), Details: "--full", Help: o.Help, Source: o.Source}
}

func inspectCompactDefinition(definition Definition) InspectCompactDefinition {
	const limit = 50
	dimensions := append([]string(nil), definition.AllowedDimensions...)
	granularities := append([]string(nil), definition.AllowedGranularities...)
	result := InspectCompactDefinition{
		LUID: definition.LUID, Name: definition.Name, DatasourceLUID: definition.DatasourceLUID,
		MeasureField: definition.MeasureField, Aggregation: definition.Aggregation, TimeDimension: definition.TimeDimension,
		RunningTotal: definition.RunningTotal, Temporality: definition.Temporality,
		DimensionsOmitted: max(0, len(dimensions)-limit), GranularitiesOmitted: max(0, len(granularities)-limit),
	}
	result.AllowedDimensions = dimensions[:min(limit, len(dimensions))]
	result.AllowedGranularities = granularities[:min(limit, len(granularities))]
	return result
}

// FullOutput returns the normalized released configuration.
func (o InspectOutput) FullOutput() any {
	item := o.Definition
	item.AllowedDimensions = append([]string(nil), item.AllowedDimensions...)
	item.AllowedGranularities = append([]string(nil), item.AllowedGranularities...)
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Definition: item, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func InspectValidateInput(input InspectInput) error {
	if strings.TrimSpace(input.LUID) == "" {
		return inspectDefinitionError("pulse.definition.inspect.usage", errs.KindUsage, input, "Pulse definition inspect requires an exact LUID.", nil)
	}
	return nil
}
