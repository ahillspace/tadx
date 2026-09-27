package definition

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
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

var createCurrencyCodePattern = regexp.MustCompile(`^[A-Z]{3}$`)

// FieldValidator verifies exact field identities against current datasource metadata.
type CreateFieldValidator interface {
	ResolveDefinitionFields(context.Context, CreateFieldReferences) (CreateFieldReferences, error)
}

// CollisionFinder finds exact name collisions within one datasource.
type CreateCollisionFinder interface {
	FindDefinitions(context.Context, string, string) ([]Definition, error)
}

// Creator creates one definition and performs bounded default-metric resolution.
type CreateCreator interface {
	CreateDefinition(context.Context, CreateRequest) (CreateResult, error)
}

// createPlan resolves live field and collision state for the validated request.
func createPlan(ctx context.Context, validator CreateFieldValidator, finder CreateCollisionFinder, input CreateInput) (CreatePlan, error) {
	request := input.request
	references := CreateFieldReferences{
		DatasourceLUID:    request.Specification.Datasource.ID,
		MeasureField:      request.Specification.BasicSpecification.Measure.Field,
		Aggregation:       request.Specification.BasicSpecification.Measure.Aggregation,
		TimeDimension:     request.Specification.BasicSpecification.TimeDimension.Field,
		AllowedDimensions: append([]string(nil), request.ExtensionOptions.AllowedDimensions...),
	}
	references, err := validator.ResolveDefinitionFields(ctx, references)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect current datasource fields, then review a new definition preview.")
		return CreatePlan{}, &errs.Error{ID: "pulse.definition.create.fields", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition field validation failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted}
	}
	request.Specification.BasicSpecification.Measure.Field = references.MeasureField
	request.Specification.BasicSpecification.TimeDimension.Field = references.TimeDimension
	request.ExtensionOptions.AllowedDimensions, err = createCanonicalIdentifiers(references.AllowedDimensions)
	if err != nil {
		return CreatePlan{}, createError("pulse.definition.create.fields", errs.KindOperation, input, "Resolved Pulse dimensions are invalid.", err)
	}
	if err := createCheckCollision(ctx, finder, input, request); err != nil {
		return CreatePlan{}, err
	}
	fingerprint, err := createRequestFingerprint(request)
	if err != nil {
		return CreatePlan{}, err
	}
	return CreatePlan{
		Mode: "preview", Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Name: request.Name,
		Datasource: request.Specification.Datasource.ID, Measure: request.Specification.BasicSpecification.Measure,
		TimeField:   request.Specification.BasicSpecification.TimeDimension.Field,
		Dimensions:  append(make([]string, 0, len(request.ExtensionOptions.AllowedDimensions)), request.ExtensionOptions.AllowedDimensions...),
		Fingerprint: fingerprint, Request: request,
	}, nil
}

func createValidated(ctx context.Context, creator CreateCreator, input CreateInput, request CreateRequest) (CreateResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateResult{}, err
	}
	result, err := creator.CreateDefinition(ctx, request)
	if err != nil {
		if result.DefinitionLUID != "" {
			result.Status = "created"
			result.DefaultMetricStatus = "unresolved"
			return result, &errs.Error{ID: "pulse.definition.create.verification_failed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Resource: result.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: "The Pulse definition was created, but its default metric could not be resolved.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", result.DefinitionLUID) + "; do not repeat the confirmed create.", TableauRequestID: result.TableauRequestID, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
		}
		return CreateResult{}, &errs.Error{ID: "pulse.definition.create.failed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition creation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: createFailureAdvice(input, err), TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	if result.DefinitionLUID == "" || result.DefaultMetricLUID == "" {
		if result.DefinitionLUID != "" {
			result.Status = "created"
			result.DefaultMetricStatus = "unresolved"
			return result, &errs.Error{ID: "pulse.definition.create.verification_failed", Kind: errs.KindOperation, Operation: "pulse.definition.create", Resource: result.DefinitionLUID, Environment: input.Environment, Site: input.Site, Summary: "The Pulse definition was created, but its default metric was not identified.", Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", result.DefinitionLUID) + "; do not repeat the confirmed create.", TableauRequestID: result.TableauRequestID, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
		}
		return CreateResult{}, &errs.Error{ID: "pulse.definition.create.invalid_response", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Tableau returned an incomplete Pulse definition creation result.", Cause: errors.New("definition and default metric LUIDs are required"), Retryable: errs.Bool(false), CorrectiveAction: "Reconcile the remote definition outcome before retrying; no complete authoritative identity was returned.", Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	return result, nil
}

func createFailureAdvice(input CreateInput, cause error) string {
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

// Create plans every call and creates unless preview is requested.
func Create(ctx context.Context, validator CreateFieldValidator, finder CreateCollisionFinder, creator CreateCreator, input CreateInput, preview bool) (CreateOutput, error) {
	plan, err := createPlan(ctx, validator, finder, input)
	if err != nil {
		return CreateOutput{}, err
	}
	output := CreateOutput{Plan: plan, Help: []string{"Run without --preview to create this Pulse definition."}}
	if createCompactPlan(plan).RequiresFull {
		output.Help = []string{"Review the complete definition with --full or the last result with --full before executing this Pulse definition create."}
	}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	result, err := createValidated(ctx, creator, input, plan.Request)
	if err != nil {
		if result.DefinitionLUID != "" {
			output.Result = &result
			output.Help = []string{commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", result.DefinitionLUID)}
			return output, err
		}
		return CreateOutput{}, err
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", result.DefaultMetricLUID)}
	return output, nil
}

func createCheckCollision(ctx context.Context, finder CreateCollisionFinder, input CreateInput, request CreateRequest) error {
	items, err := finder.FindDefinitions(ctx, request.Name, request.Specification.Datasource.ID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review current Pulse definitions, then request a new preview.")
		return &errs.Error{ID: "pulse.definition.create.collision", Kind: errs.KindOperation, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: "Pulse definition collision check failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted}
	}
	for _, item := range items {
		if item.Name == request.Name && item.DatasourceLUID == request.Specification.Datasource.ID {
			return &errs.Error{ID: "pulse.definition.create.conflict", Kind: errs.KindOperation, Operation: "pulse.definition.create", Resource: item.LUID, Environment: input.Environment, Site: input.Site, Summary: "A Pulse definition with this name already exists for the datasource.", Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "definition", "inspect", "--id", item.LUID) + "; inspect the existing definition or choose a different name.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
		}
	}
	return nil
}

func createRequestFromIntent(intent CreateIntent) (CreateRequest, error) {
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
	aggregation, ok := createEnumValue(intent.Aggregation, "SUM", map[string]string{
		"SUM": "AGGREGATION_SUM", "AVERAGE": "AGGREGATION_AVERAGE", "MIN": "AGGREGATION_MIN", "MAX": "AGGREGATION_MAX",
		"COUNT": "AGGREGATION_COUNT", "COUNT_DISTINCT": "AGGREGATION_COUNT_DISTINCT", "USER": "AGGREGATION_USER",
	})
	if !ok {
		return CreateRequest{}, errors.New("aggregation must be SUM, AVERAGE, MIN, MAX, COUNT, COUNT_DISTINCT, or USER")
	}
	format, ok := createEnumValue(intent.NumberFormat, "NUMBER", map[string]string{"NUMBER": "NUMBER_FORMAT_TYPE_NUMBER", "CURRENCY": "NUMBER_FORMAT_TYPE_CURRENCY", "PERCENT": "NUMBER_FORMAT_TYPE_PERCENT"})
	if !ok {
		return CreateRequest{}, errors.New("number format must be NUMBER, CURRENCY, or PERCENT")
	}
	sentiment, ok := createEnumValue(intent.Sentiment, "NONE", map[string]string{"UP": "SENTIMENT_TYPE_UP_IS_GOOD", "DOWN": "SENTIMENT_TYPE_DOWN_IS_GOOD", "NONE": "SENTIMENT_TYPE_NONE"})
	if !ok {
		return CreateRequest{}, errors.New("sentiment must be UP, DOWN, or NONE")
	}
	temporality, ok := createEnumValue(intent.Temporality, "OVER_TIME", map[string]string{"OVER_TIME": "TEMPORALITY_OVER_TIME", "LATEST": "TEMPORALITY_LATEST_POINT_IN_TIME"})
	if !ok {
		return CreateRequest{}, errors.New("temporality must be OVER_TIME or LATEST")
	}
	if err := pulsecontract.ValidateRunningTotal(aggregation, temporality, intent.RunningTotal); err != nil {
		return CreateRequest{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(intent.CurrencyCode))
	if format == "NUMBER_FORMAT_TYPE_CURRENCY" {
		if currency == "" {
			currency = "USD"
		}
		if !createCurrencyCodePattern.MatchString(currency) {
			return CreateRequest{}, errors.New("currency code must contain three uppercase letters")
		}
	} else if currency != "" {
		return CreateRequest{}, errors.New("currency code requires CURRENCY number format")
	}
	dimensions, err := createCanonicalIdentifiers(intent.AllowedDimensions)
	if err != nil {
		return CreateRequest{}, err
	}
	if len(dimensions) == 0 {
		return CreateRequest{}, errors.New("at least one adjustable dimension is required; provide --dimension with an eligible exact field ID")
	}
	granularities, err := createAllowedGranularities(intent.MinimumGranularity)
	if err != nil {
		return CreateRequest{}, err
	}
	representation := CreateRepresentationOptions{Type: format, SentimentType: sentiment}
	if format == "NUMBER_FORMAT_TYPE_CURRENCY" {
		representation.CurrencyCode = "CURRENCY_CODE_" + currency
	}
	return CreateRequest{
		Name: intent.Name, Description: intent.Description,
		Specification: CreateSpecification{
			Datasource:         CreateDatasource{ID: intent.DatasourceLUID},
			BasicSpecification: CreateBasicSpecification{Measure: CreateMeasure{Field: intent.MeasureField, Aggregation: aggregation}, TimeDimension: CreateTimeDimension{Field: intent.TimeDimension}, Filters: []CreateFilter{}},
			RunningTotal:       intent.RunningTotal, Temporality: temporality,
		},
		ExtensionOptions:      CreateExtensionOptions{AllowedDimensions: dimensions, AllowedGranularities: granularities, OffsetFromToday: 0, UseDynamicOffset: false},
		RepresentationOptions: representation,
		InsightsOptions:       CreateInsightsOptions{ShowInsights: true, Settings: createDefaultInsightSettings()},
		Comparisons: CreateComparisons{Comparisons: []CreateComparison{
			{CompareConfig: CreateCompareConfig{Comparison: "TIME_COMPARISON_PREVIOUS_PERIOD"}, Index: 0},
			{CompareConfig: CreateCompareConfig{Comparison: "TIME_COMPARISON_YEAR_AGO_PERIOD"}, Index: 1},
		}},
		DatasourceGoals: []map[string]any{}, RelatedLinks: []map[string]any{}, Certification: CreateCertification{IsCertified: false},
	}, nil
}

func createEnumValue(value, defaultValue string, values map[string]string) (string, bool) {
	key := strings.ToUpper(strings.TrimSpace(value))
	if key == "" {
		key = defaultValue
	}
	result, ok := values[key]
	return result, ok
}

func createRequestFingerprint(request CreateRequest) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("encode Pulse definition request: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func createCanonicalIdentifiers(values []string) ([]string, error) {
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

func createAllowedGranularities(value string) ([]string, error) {
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

func createDefaultInsightSettings() []CreateInsightSetting {
	types := []string{
		"INSIGHT_TYPE_RISKY_MONOPOLY", "INSIGHT_TYPE_TOP_DRIVERS", "INSIGHT_TYPE_CURRENT_TREND",
		"INSIGHT_TYPE_BOTTOM_CONTRIBUTORS", "INSIGHT_TYPE_TOP_DETRACTORS", "INSIGHT_TYPE_NEW_TREND",
		"INSIGHT_TYPE_UNUSUAL_CHANGE", "INSIGHT_TYPE_RECORD_LEVEL_OUTLIERS", "INSIGHT_TYPE_CORRELATED_METRIC", "INSIGHT_TYPE_METRIC_FORECAST",
	}
	settings := make([]CreateInsightSetting, len(types))
	for index, insightType := range types {
		settings[index] = CreateInsightSetting{Type: insightType, Disabled: insightType == "INSIGHT_TYPE_RECORD_LEVEL_OUTLIERS"}
	}
	return settings
}

func createError(id string, kind errs.Kind, input CreateInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.create", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Correct the Pulse definition intent and review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}

// ValidateInput checks the complete authoring intent without live field validation.
func CreateValidateInput(input *CreateInput) error {
	request, err := createRequestFromIntent(input.Intent)
	if err != nil {
		return createError("pulse.definition.create.usage", errs.KindUsage, *input, "Pulse definition intent is invalid.", err)
	}
	input.request = request
	return nil
}
