package create

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

var currencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

// FieldValidator verifies exact field identities against current datasource metadata.
type FieldValidator interface {
	ResolveDefinitionFields(context.Context, FieldReferences) (FieldReferences, error)
	ValidateDefinitionFields(context.Context, FieldReferences) error
}

// CollisionFinder finds exact name collisions within one datasource.
type CollisionFinder interface {
	FindDefinitions(context.Context, string, string) ([]ExistingDefinition, error)
}

// Creator creates one definition and performs bounded default-metric resolution.
type Creator interface {
	CreateDefinition(context.Context, CreateRequest) (CreateResult, error)
}

// Action previews and creates one Pulse definition.
type Action struct {
	validator FieldValidator
	finder    CollisionFinder
	creator   Creator
}

// New creates a Pulse definition create action.
func New(validator FieldValidator, finder CollisionFinder, creator Creator) *Action {
	return &Action{validator: validator, finder: finder, creator: creator}
}

// Plan validates the small intent and resolves live field and collision state.
func (a *Action) Plan(ctx context.Context, input Input) (Plan, error) {
	if err := ValidateInput(input); err != nil {
		return Plan{}, err
	}
	if a == nil || a.validator == nil || a.finder == nil || a.creator == nil {
		return Plan{}, createError("pulse.definition.create.unconfigured", errs.KindRuntime, input, "Pulse definition creation is not configured.", nil)
	}
	request, err := requestFromIntent(input.Intent)
	if err != nil {
		return Plan{}, createError("pulse.definition.create.usage", errs.KindUsage, input, "Pulse definition intent is invalid.", err)
	}
	references := FieldReferences{
		DatasourceLUID:    request.Specification.Datasource.ID,
		MeasureField:      request.Specification.BasicSpecification.Measure.Field,
		Aggregation:       request.Specification.BasicSpecification.Measure.Aggregation,
		TimeDimension:     request.Specification.BasicSpecification.TimeDimension.Field,
		AllowedDimensions: append([]string(nil), request.ExtensionOptions.AllowedDimensions...),
	}
	references, err = a.validator.ResolveDefinitionFields(ctx, references)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect current datasource fields, then review a new definition preview.")
		return Plan{}, &errs.Error{ID: "pulse.definition.create.fields", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition field validation failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	request.Specification.BasicSpecification.Measure.Field = references.MeasureField
	request.Specification.BasicSpecification.TimeDimension.Field = references.TimeDimension
	request.ExtensionOptions.AllowedDimensions, err = canonicalIdentifiers(references.AllowedDimensions)
	if err != nil {
		return Plan{}, createError("pulse.definition.create.fields", errs.KindOperation, input, "Resolved Pulse dimensions are invalid.", err)
	}
	if err := a.checkCollision(ctx, input, request); err != nil {
		return Plan{}, err
	}
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Mode: "preview", Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Name: request.Name,
		Datasource: request.Specification.Datasource.ID, Measure: request.Specification.BasicSpecification.Measure,
		TimeField:   request.Specification.BasicSpecification.TimeDimension.Field,
		Dimensions:  append(make([]string, 0, len(request.ExtensionOptions.AllowedDimensions)), request.ExtensionOptions.AllowedDimensions...),
		Fingerprint: fingerprint, Request: request, planned: true,
	}, nil
}

// Apply revalidates live state and performs the exact planned request.
func (a *Action) Apply(ctx context.Context, input Input, plan Plan) (CreateResult, error) {
	if a == nil || a.validator == nil || a.finder == nil || a.creator == nil {
		return CreateResult{}, createError("pulse.definition.create.unconfigured", errs.KindRuntime, input, "Pulse definition creation is not configured.", nil)
	}
	if !plan.planned || plan.Operation != "pulse.definition.create" || plan.Request.Name == "" || plan.Fingerprint == "" {
		return CreateResult{}, createError("pulse.definition.create.usage", errs.KindUsage, input, "Pulse definition apply requires a plan produced by this action.", nil)
	}
	fingerprint, err := requestFingerprint(plan.Request)
	if err != nil || fingerprint != plan.Fingerprint {
		return CreateResult{}, createError("pulse.definition.create.usage", errs.KindUsage, input, "Pulse definition apply requires an unchanged plan produced by this action.", err)
	}
	references := FieldReferences{
		DatasourceLUID:    plan.Request.Specification.Datasource.ID,
		MeasureField:      plan.Request.Specification.BasicSpecification.Measure.Field,
		Aggregation:       plan.Request.Specification.BasicSpecification.Measure.Aggregation,
		TimeDimension:     plan.Request.Specification.BasicSpecification.TimeDimension.Field,
		AllowedDimensions: append([]string(nil), plan.Request.ExtensionOptions.AllowedDimensions...),
	}
	if err := a.validator.ValidateDefinitionFields(ctx, references); err != nil {
		return CreateResult{}, &errs.Error{ID: "pulse.definition.create.target_changed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition field state changed during revalidation.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect current datasource fields, then review a new definition preview.", TableauRequestID: errs.TableauRequestID(err)}
	}
	if err := a.checkCollision(ctx, input, plan.Request); err != nil {
		return CreateResult{}, err
	}
	return a.createValidated(ctx, input, plan.Request)
}

// createValidated is private to fresh Plan/Execute and revalidated Apply flows.
// It never accepts a caller-supplied retained plan without Apply's drift checks.
func (a *Action) createValidated(ctx context.Context, input Input, request CreateRequest) (CreateResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateResult{}, err
	}
	result, err := a.creator.CreateDefinition(ctx, request)
	if err != nil {
		if result.DefinitionLUID != "" {
			result.Status = "created"
			result.DefaultMetricStatus = "unresolved"
			return result, &errs.Error{ID: "pulse.definition.create.verification_failed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Resource: result.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: "The Pulse definition was created, but its default metric could not be resolved.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", result.DefinitionLUID) + "; do not repeat the confirmed create.", TableauRequestID: result.TableauRequestID}
		}
		return CreateResult{}, &errs.Error{ID: "pulse.definition.create.failed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition creation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: createFailureAdvice(input, err), TableauRequestID: errs.TableauRequestID(err)}
	}
	if result.DefinitionLUID == "" || result.DefaultMetricLUID == "" {
		if result.DefinitionLUID != "" {
			result.Status = "created"
			result.DefaultMetricStatus = "unresolved"
			return result, &errs.Error{ID: "pulse.definition.create.verification_failed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Resource: result.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: "The Pulse definition was created, but its default metric was not identified.", Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", result.DefinitionLUID) + "; do not repeat the confirmed create.", TableauRequestID: result.TableauRequestID}
		}
		return CreateResult{}, createError("pulse.definition.create.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete Pulse definition creation result.", errors.New("definition and default metric LUIDs are required"))
	}
	return result, nil
}

func createFailureAdvice(input Input, cause error) string {
	lookup := "Run " + commandhint.Environment(input.Environment, "pulse", "definition", "list", "--datasource-id", input.Intent.DatasourceLUID, "--full") + ", then inspect candidate definitions by exact LUID in that environment. Compare datasource, measure, aggregation, date field, filters, and dimensions before another create."
	var status interface{ HTTPStatus() int }
	if errors.As(cause, &status) {
		switch status.HTTPStatus() {
		case 400:
			return "Review the Tableau error details and the same create command with --preview --full. " + lookup
		case 409:
			return "Check whether an equivalent definition exists, including definitions with different names; HTTP 409 alone does not establish the conflict cause. " + lookup
		case 401, 403:
			_, advice := errs.CompleteRetryAdvice(cause, "Verify authentication and access in the selected environment.")
			return advice + " " + lookup
		}
	}
	return "Reconcile the remote outcome before attempting another create. " + lookup
}

// Execute plans every call and creates unless preview is requested.
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	plan, err := a.Plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	output := Output{Plan: plan, Help: []string{"Run without --preview to create this Pulse definition."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	// No asynchronous work or external caller intervenes after this fresh Plan.
	// Retained plans must still use public Apply and its current-state validation.
	result, err := a.createValidated(ctx, input, plan.Request)
	if err != nil {
		if result.DefinitionLUID != "" {
			output.Result = &result
			output.Help = []string{commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", result.DefinitionLUID)}
			return output, err
		}
		return Output{}, err
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", result.DefaultMetricLUID)}
	return output, nil
}

func (a *Action) checkCollision(ctx context.Context, input Input, request CreateRequest) error {
	items, err := a.finder.FindDefinitions(ctx, request.Name, request.Specification.Datasource.ID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review current Pulse definitions, then request a new preview.")
		return &errs.Error{ID: "pulse.definition.create.collision", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition collision check failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	for _, item := range items {
		if item.Name == request.Name && item.DatasourceLUID == request.Specification.Datasource.ID {
			return &errs.Error{ID: "pulse.definition.create.conflict", Kind: errs.KindOperation, Operation: "pulse.definition.create", Resource: item.LUID, Environment: input.Environment, Site: input.Site, Summary: "A Pulse definition with this name already exists for the datasource.", Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", item.LUID) + "; inspect the existing definition or choose a different name."}
		}
	}
	return nil
}

func requestFromIntent(intent Intent) (CreateRequest, error) {
	intent.Name = strings.TrimSpace(intent.Name)
	intent.Description = strings.TrimSpace(intent.Description)
	intent.DatasourceLUID = strings.TrimSpace(intent.DatasourceLUID)
	intent.MeasureField = strings.TrimSpace(intent.MeasureField)
	intent.TimeDimension = strings.TrimSpace(intent.TimeDimension)
	if intent.Name == "" || intent.DatasourceLUID == "" || intent.MeasureField == "" || intent.TimeDimension == "" {
		return CreateRequest{}, errors.New("name, datasource LUID, measure field, and time dimension are required")
	}
	if utf8.RuneCountInString(intent.Name) > 255 || utf8.RuneCountInString(intent.Description) > 1024 {
		return CreateRequest{}, errors.New("name or description exceeds the Pulse length limit")
	}
	aggregation, ok := enumValue(intent.Aggregation, "SUM", map[string]string{
		"SUM": "AGGREGATION_SUM", "AVERAGE": "AGGREGATION_AVERAGE", "MIN": "AGGREGATION_MIN", "MAX": "AGGREGATION_MAX",
		"COUNT": "AGGREGATION_COUNT", "COUNT_DISTINCT": "AGGREGATION_COUNT_DISTINCT", "USER": "AGGREGATION_USER",
	})
	if !ok {
		return CreateRequest{}, errors.New("aggregation must be SUM, AVERAGE, MIN, MAX, COUNT, COUNT_DISTINCT, or USER")
	}
	format, ok := enumValue(intent.NumberFormat, "NUMBER", map[string]string{"NUMBER": "NUMBER_FORMAT_TYPE_NUMBER", "CURRENCY": "NUMBER_FORMAT_TYPE_CURRENCY", "PERCENT": "NUMBER_FORMAT_TYPE_PERCENT"})
	if !ok {
		return CreateRequest{}, errors.New("number format must be NUMBER, CURRENCY, or PERCENT")
	}
	sentiment, ok := enumValue(intent.Sentiment, "NONE", map[string]string{"UP": "SENTIMENT_TYPE_UP_IS_GOOD", "DOWN": "SENTIMENT_TYPE_DOWN_IS_GOOD", "NONE": "SENTIMENT_TYPE_NONE"})
	if !ok {
		return CreateRequest{}, errors.New("sentiment must be UP, DOWN, or NONE")
	}
	temporality, ok := enumValue(intent.Temporality, "OVER_TIME", map[string]string{"OVER_TIME": "TEMPORALITY_OVER_TIME", "LATEST": "TEMPORALITY_LATEST_POINT_IN_TIME"})
	if !ok {
		return CreateRequest{}, errors.New("temporality must be OVER_TIME or LATEST")
	}
	if intent.RunningTotal && (aggregation != "AGGREGATION_SUM" || temporality != "TEMPORALITY_OVER_TIME") {
		return CreateRequest{}, errors.New("running total requires SUM aggregation and OVER_TIME temporality")
	}
	currency := strings.ToUpper(strings.TrimSpace(intent.CurrencyCode))
	if format == "NUMBER_FORMAT_TYPE_CURRENCY" {
		if currency == "" {
			currency = "USD"
		}
		if !currencyCodePattern.MatchString(currency) {
			return CreateRequest{}, errors.New("currency code must contain three uppercase letters")
		}
	} else if currency != "" {
		return CreateRequest{}, errors.New("currency code requires CURRENCY number format")
	}
	dimensions, err := canonicalIdentifiers(intent.AllowedDimensions)
	if err != nil {
		return CreateRequest{}, err
	}
	if len(dimensions) == 0 {
		return CreateRequest{}, errors.New("at least one adjustable dimension is required; provide --dimension with an eligible exact field ID")
	}
	granularities, err := allowedGranularities(intent.MinimumGranularity)
	if err != nil {
		return CreateRequest{}, err
	}
	representation := RepresentationOptions{Type: format, SentimentType: sentiment}
	if format == "NUMBER_FORMAT_TYPE_CURRENCY" {
		representation.CurrencyCode = "CURRENCY_CODE_" + currency
	}
	return CreateRequest{
		Name: intent.Name, Description: intent.Description,
		Specification: Specification{
			Datasource:         Datasource{ID: intent.DatasourceLUID},
			BasicSpecification: BasicSpecification{Measure: Measure{Field: intent.MeasureField, Aggregation: aggregation}, TimeDimension: TimeDimension{Field: intent.TimeDimension}, Filters: []Filter{}},
			RunningTotal:       intent.RunningTotal, Temporality: temporality,
		},
		ExtensionOptions:      ExtensionOptions{AllowedDimensions: dimensions, AllowedGranularities: granularities, OffsetFromToday: 0, UseDynamicOffset: false},
		RepresentationOptions: representation,
		InsightsOptions:       InsightsOptions{ShowInsights: true, Settings: defaultInsightSettings()},
		Comparisons: Comparisons{Comparisons: []Comparison{
			{CompareConfig: CompareConfig{Comparison: "TIME_COMPARISON_PREVIOUS_PERIOD"}, Index: 0},
			{CompareConfig: CompareConfig{Comparison: "TIME_COMPARISON_YEAR_AGO_PERIOD"}, Index: 1},
		}},
		DatasourceGoals: []map[string]any{}, RelatedLinks: []map[string]any{}, Certification: Certification{IsCertified: false},
	}, nil
}

func enumValue(value, defaultValue string, values map[string]string) (string, bool) {
	key := strings.ToUpper(strings.TrimSpace(value))
	if key == "" {
		key = defaultValue
	}
	result, ok := values[key]
	return result, ok
}

func requestFingerprint(request CreateRequest) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode Pulse definition request: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func canonicalIdentifiers(values []string) ([]string, error) {
	unique := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("allowed dimensions cannot contain blank field identifiers")
		}
		if _, exists := unique[value]; exists {
			continue
		}
		unique[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func allowedGranularities(value string) ([]string, error) {
	all := []string{"GRANULARITY_BY_DAY", "GRANULARITY_BY_WEEK", "GRANULARITY_BY_MONTH", "GRANULARITY_BY_QUARTER", "GRANULARITY_BY_YEAR"}
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "DAY":
		return all, nil
	case "WEEK":
		return all[1:], nil
	case "MONTH":
		return all[2:], nil
	case "QUARTER":
		return all[3:], nil
	case "YEAR":
		return all[4:], nil
	default:
		return nil, errors.New("minimum granularity must be DAY, WEEK, MONTH, QUARTER, or YEAR")
	}
}

func defaultInsightSettings() []InsightSetting {
	types := []string{
		"INSIGHT_TYPE_RISKY_MONOPOLY", "INSIGHT_TYPE_TOP_DRIVERS", "INSIGHT_TYPE_CURRENT_TREND",
		"INSIGHT_TYPE_BOTTOM_CONTRIBUTORS", "INSIGHT_TYPE_TOP_DETRACTORS", "INSIGHT_TYPE_NEW_TREND",
		"INSIGHT_TYPE_UNUSUAL_CHANGE", "INSIGHT_TYPE_RECORD_LEVEL_OUTLIERS", "INSIGHT_TYPE_CORRELATED_METRIC", "INSIGHT_TYPE_METRIC_FORECAST",
	}
	settings := make([]InsightSetting, len(types))
	for index, insightType := range types {
		settings[index] = InsightSetting{Type: insightType, Disabled: insightType == "INSIGHT_TYPE_RECORD_LEVEL_OUTLIERS"}
	}
	return settings
}

func createError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Correct the Pulse definition intent and review a new preview."}
}
