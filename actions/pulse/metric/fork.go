package metric

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

type ForkReader interface {
	ResolveFilterFields(context.Context, string, []string) ([]string, error)
	GetMetric(context.Context, string) (Metric, error)
	GetDefinition(context.Context, string) (ForkDefinition, error)
}
type ForkCreator interface {
	GetOrCreateMetric(context.Context, ForkCreateRequest) (ForkCreateResult, error)
}
type ForkReconciler interface {
	ReconcileMetric(context.Context, ForkExpectedMetric) (ForkReconciliation, error)
}

func runFork(ctx context.Context, reader ForkReader, creator ForkCreator, reconciler ForkReconciler, input ForkInput, preview bool) (ForkOutput, error) {
	plan, err := forkPlan(ctx, reader, input)
	if err != nil {
		return ForkOutput{}, err
	}
	output := ForkOutput{Plan: plan}
	if preview {
		output.Help = []string{"Run without --preview to get or create this exact Pulse metric variant."}
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := forkPlan(ctx, reader, input)
	if err != nil {
		return ForkOutput{}, err
	}
	if current.Fingerprint != plan.Fingerprint {
		return ForkOutput{}, forkFail("pulse.metric.fork.changed", errs.KindOperation, input, "The source Pulse metric changed during revalidation.", errors.New("planned specification fingerprint changed"))
	}
	created, err := creator.GetOrCreateMetric(ctx, ForkCreateRequest{DefinitionLUID: plan.DefinitionLUID, Specification: forkCloneMap(plan.Specification)})
	if err != nil {
		if created.MetricLUID != "" {
			output.Result = &ForkResult{Status: forkStatus(created.Created), MetricLUID: created.MetricLUID, MetricName: created.MetricName, Created: created.Created, RequestID: created.RequestID}
		}
		output.Help = forkExecutionRecoveryHelp(input, created.MetricLUID)
		retryable, corrective := errs.CompleteRetryAdvice(err, commandhint.Environment(input.Environment, "pulse", "metric", "list", "--definition-id", plan.DefinitionLUID, "--all")+"; reconcile the remote outcome before retrying.")
		return output, &errs.Error{ID: "pulse.metric.fork.failed", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric fork failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	if created.MetricLUID == "" {
		output.Help = forkExecutionRecoveryHelp(input, "")
		return output, &errs.Error{ID: "pulse.metric.fork.invalid_response", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Tableau returned no metric identity for the fork.", Retryable: errs.Bool(false), CorrectiveAction: "Reconcile the remote fork outcome before retrying; no authoritative metric identity was returned.", Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	reconciled, err := reconciler.ReconcileMetric(ctx, ForkExpectedMetric{MetricLUID: created.MetricLUID, DefinitionLUID: plan.DefinitionLUID, DatasourceLUID: plan.DatasourceLUID, SiteLUID: input.SiteLUID, Specification: forkCloneMap(plan.Specification)})
	if err != nil {
		output.Result = &ForkResult{Status: forkStatus(created.Created), MetricLUID: created.MetricLUID, MetricName: created.MetricName, Created: created.Created, ReconciliationStatus: "unknown", RequestID: created.RequestID}
		output.Help = forkExecutionRecoveryHelp(input, created.MetricLUID)
		return output, &errs.Error{ID: "pulse.metric.fork.reconcile", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: created.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric reconciliation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", created.MetricLUID) + "; reconcile before retrying.", TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
	}
	if reconciled.Status != "verified" || !reconciled.OwnershipVerified || !reconciled.SpecificationVerified {
		output.Result = &ForkResult{Status: forkStatus(created.Created), MetricLUID: created.MetricLUID, MetricName: created.MetricName, Created: created.Created, ReconciliationStatus: reconciled.Status, RequestID: created.RequestID, ReconciliationRequestID: reconciled.RequestID}
		output.Help = forkExecutionRecoveryHelp(input, created.MetricLUID)
		return output, &errs.Error{ID: "pulse.metric.fork.reconcile", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: created.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "The forked Pulse metric's saved configuration could not be verified.", Cause: fmt.Errorf("reconciliation status %s", reconciled.Status), Retryable: errs.Bool(false), CorrectiveAction: commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", created.MetricLUID) + "; do not repeat the mutation automatically.", TableauRequestID: reconciled.RequestID, Phase: errs.PhaseVerification, Outcome: errs.OutcomeConfirmed}
	}
	status := forkStatus(created.Created)
	output.Result = &ForkResult{Status: status, MetricLUID: created.MetricLUID, MetricName: created.MetricName, Created: created.Created, ReconciliationStatus: reconciled.Status, ReconciliationAttempts: reconciled.Attempts, OwnershipVerified: reconciled.OwnershipVerified, RequestID: created.RequestID, ReconciliationRequestID: reconciled.RequestID}
	output.Result.SpecificationVerified = reconciled.SpecificationVerified
	output.Result.SavedSpecification = forkCloneMap(reconciled.SavedSpecification)
	output.Result.SavedDefinition = reconciled.SavedDefinition
	output.Result.MetricReadbackRequestID = reconciled.MetricRequestID
	output.Result.DefinitionReadbackRequestID = reconciled.DefinitionRequestID
	output.Help = []string{"Saved metric configuration and definition linkage verified; current values and generated insights are not read by TADX."}
	return output, nil
}

func forkExecutionRecoveryHelp(input ForkInput, metricLUID string) []string {
	if metricLUID != "" {
		return []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", metricLUID, "--full")}
	}
	return []string{"The Pulse metric fork outcome is unknown; reconcile the remote result before retrying."}
}

func forkStatus(created bool) string {
	if created {
		return "created"
	}
	return "existing"
}
func forkPlan(ctx context.Context, reader ForkReader, input ForkInput) (ForkPlan, error) {
	metric, err := reader.GetMetric(ctx, input.MetricLUID)
	if err != nil {
		return ForkPlan{}, forkReadFail(input, "Pulse source metric retrieval failed.", err)
	}
	if metric.LUID != input.MetricLUID || metric.DefinitionLUID == "" || metric.Specification == nil {
		return ForkPlan{}, forkFail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "Tableau returned an incomplete or mismatched source metric.", nil)
	}
	if input.SiteLUID != "" && metric.SiteLUID != "" && metric.SiteLUID != input.SiteLUID {
		return ForkPlan{}, forkFail("pulse.metric.fork.ownership", errs.KindOperation, input, "The source metric belongs to a different Tableau site.", nil)
	}
	definition, err := reader.GetDefinition(ctx, metric.DefinitionLUID)
	if err != nil {
		return ForkPlan{}, forkReadFail(input, "Pulse source definition retrieval failed.", err)
	}
	if definition.LUID != metric.DefinitionLUID || definition.DatasourceLUID == "" {
		return ForkPlan{}, forkFail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "Tableau returned an incomplete or mismatched source definition.", nil)
	}
	spec := forkCloneMap(metric.Specification)
	delete(spec, "datasource")
	if input.Timeframe != "" {
		spec["measurement_period"] = input.period
	}
	if len(definition.AllowedGranularities) == 0 {
		return ForkPlan{}, forkFail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "The source definition omitted allowed granularities; its supported periods cannot be validated.", nil)
	}
	period, ok := spec["measurement_period"].(map[string]any)
	if !ok || forkStringValue(period["granularity"]) == "" {
		return ForkPlan{}, forkFail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "The source metric omitted its period granularity; provide --period to select a supported period.", nil)
	}
	granularity := forkStringValue(period["granularity"])
	if !slices.Contains(definition.AllowedGranularities, granularity) {
		return ForkPlan{}, forkFail("pulse.metric.fork.usage", errs.KindUsage, input, "The metric period granularity is not allowed by its definition.", fmt.Errorf("granularity %s is not supported; allowed granularities: %s", granularity, strings.Join(definition.AllowedGranularities, ", ")))
	}
	allowed := map[string]bool{}
	for _, field := range definition.AllowedDimensions {
		allowed[field] = true
	}
	filters := slices.Clone(input.Filters)
	for i := range filters {
		filters[i].Values = slices.Clone(filters[i].Values)
	}
	selectors := make([]string, len(filters))
	needsResolution := false
	for i, filter := range filters {
		selectors[i] = filter.Field
		needsResolution = needsResolution || !allowed[filter.Field]
	}
	if needsResolution {
		resolved, resolveErr := reader.ResolveFilterFields(ctx, definition.DatasourceLUID, selectors)
		if resolveErr != nil {
			return ForkPlan{}, forkFail("pulse.metric.fork.fields", errs.KindOperation, input, "Pulse filter field resolution failed.", resolveErr)
		}
		if len(resolved) != len(filters) {
			return ForkPlan{}, forkFail("pulse.metric.fork.fields", errs.KindOperation, input, "Pulse filter field resolution returned incomplete identities.", nil)
		}
		for i := range filters {
			filters[i].Field = resolved[i]
		}
	}
	filters, err = forkMergeResolvedFilters(filters)
	if err != nil {
		return ForkPlan{}, forkFail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse filters have conflicting operators or invalid combined values.", err)
	}
	for _, filter := range filters {
		if filter.Field == "" || len(filter.Values) == 0 || !forkValidFilterValues(filter.Values) {
			return ForkPlan{}, forkFail("pulse.metric.fork.usage", errs.KindUsage, input, "Every dimensional filter must name an allowed field and at least one value.", nil)
		}
		if !allowed[filter.Field] {
			return ForkPlan{}, forkFail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse filter field is not allowed by the source definition.", fmt.Errorf("field %q is not among the allowed dimensions for definition %q (allowed: %s)", filter.Field, definition.LUID, strings.Join(definition.AllowedDimensions, ", ")))
		}
		spec, err = forkMergeFilter(spec, filter)
		if err != nil {
			return ForkPlan{}, forkFail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "The source Pulse metric has an unsupported filter specification.", err)
		}
	}
	data, _ := json.Marshal(struct {
		Definition             string         `json:"definition"`
		Specification          map[string]any `json:"specification"`
		DefinitionFilters      []any          `json:"definition_filters"`
		DefinitionFiltersKnown bool           `json:"definition_filters_known"`
	}{definition.LUID, spec, definition.FixedFilters, definition.FixedFiltersKnown})
	sum := sha256.Sum256(data)
	return ForkPlan{Mode: "preview", Operation: "pulse.metric.fork", Environment: input.Environment, Site: input.Site, SourceMetricLUID: input.MetricLUID, DefinitionLUID: definition.LUID, DatasourceLUID: definition.DatasourceLUID, Timeframe: input.Timeframe, Filters: filters, Specification: spec, DefinitionFilters: definition.FixedFilters, DefinitionFiltersKnown: definition.FixedFiltersKnown, Fingerprint: "sha256:" + hex.EncodeToString(sum[:])}, nil
}

func forkMergeResolvedFilters(filters []ForkFilter) ([]ForkFilter, error) {
	byField := map[string]int{}
	merged := []ForkFilter{}
	for _, filter := range filters {
		if i, ok := byField[filter.Field]; ok {
			if merged[i].Exclude != filter.Exclude {
				return nil, fmt.Errorf("field %q has conflicting include and exclude filters", filter.Field)
			}
			merged[i].Values = append(merged[i].Values, filter.Values...)
		} else {
			byField[filter.Field] = len(merged)
			merged = append(merged, filter)
		}
	}
	merged = forkCanonicalFilters(merged)
	for _, filter := range merged {
		if !forkValidFilterValues(filter.Values) {
			return nil, fmt.Errorf("field %q has invalid combined filter values", filter.Field)
		}
	}
	return merged, nil
}
func forkMeasurementPeriod(key string, days int) (map[string]any, bool) {
	simple := map[string][2]string{"TODAY": {"GRANULARITY_BY_DAY", "RANGE_CURRENT_PARTIAL"}, "THIS_WEEK": {"GRANULARITY_BY_WEEK", "RANGE_CURRENT_PARTIAL"}, "MONTH_TO_DATE": {"GRANULARITY_BY_MONTH", "RANGE_CURRENT_PARTIAL"}, "QUARTER_TO_DATE": {"GRANULARITY_BY_QUARTER", "RANGE_CURRENT_PARTIAL"}, "YEAR_TO_DATE": {"GRANULARITY_BY_YEAR", "RANGE_CURRENT_PARTIAL"}, "YESTERDAY": {"GRANULARITY_BY_DAY", "RANGE_LAST_COMPLETE"}, "LAST_WEEK": {"GRANULARITY_BY_WEEK", "RANGE_LAST_COMPLETE"}, "LAST_MONTH": {"GRANULARITY_BY_MONTH", "RANGE_LAST_COMPLETE"}, "LAST_QUARTER": {"GRANULARITY_BY_QUARTER", "RANGE_LAST_COMPLETE"}, "LAST_YEAR": {"GRANULARITY_BY_YEAR", "RANGE_LAST_COMPLETE"}}
	if pair, ok := simple[key]; ok {
		return map[string]any{"granularity": pair[0], "range": pair[1]}, true
	}
	periods := map[string]int{"LAST_7_DAYS": 7, "LAST_14_DAYS": 14, "LAST_30_DAYS": 30, "LAST_60_DAYS": 60, "LAST_90_DAYS": 90}
	period, ok := periods[key]
	if key == "CUSTOM_N_DAYS" {
		period = days
		ok = ForkIsSupportedCustomDays(days)
	}
	if !ok {
		return nil, false
	}
	return map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG", "last_x_period": map[string]any{"period": period, "period_type": "GRANULARITY_BY_DAY", "include_current_period": true}}, true
}
func forkCanonicalFilters(values []ForkFilter) []ForkFilter {
	out := make([]ForkFilter, 0, len(values))
	for _, value := range values {
		value.Field = strings.TrimSpace(value.Field)
		seen := map[string]bool{}
		clean := []string{}
		for _, entry := range value.Values {
			if entry != "" && !seen[entry] {
				seen[entry] = true
				clean = append(clean, entry)
			}
		}
		slices.Sort(clean)
		value.Values = clean
		out = append(out, value)
	}
	slices.SortFunc(out, func(left, right ForkFilter) int { return cmp.Compare(left.Field, right.Field) })
	return out
}
func forkMergeFilter(spec map[string]any, filter ForkFilter) (map[string]any, error) {
	raw, ok := spec["filters"].([]any)
	if !ok && spec["filters"] != nil {
		return nil, errors.New("source metric filters must be an array")
	}
	items := make([]any, 0, len(raw)+1)
	matches := 0
	for _, entry := range raw {
		record, ok := entry.(map[string]any)
		if ok && forkStringValue(record["field"]) == filter.Field {
			matches++
			continue
		}
		items = append(items, entry)
	}
	if matches > 1 {
		return nil, fmt.Errorf("source metric has multiple filters on %s", filter.Field)
	}
	categorical := make([]any, len(filter.Values))
	for i, value := range filter.Values {
		categorical[i] = map[string]any{"string_value": value}
	}
	operator := "OPERATOR_EQUAL"
	if filter.Exclude {
		operator = "OPERATOR_NOT_EQUAL"
	}
	items = append(items, map[string]any{"field": filter.Field, "operator": operator, "categorical_values": categorical, "include_null": false})
	spec["filters"] = items
	return spec, nil
}

func forkValidFilterValues(values []string) bool {
	if len(values) > 10000 {
		return false
	}
	for _, value := range values {
		if len([]rune(value)) > 256 {
			return false
		}
	}
	return true
}

// cloneMap deep-copies a decoded JSON object while preserving numeric fidelity.
// It decodes with UseNumber so integers and large numbers survive the round-trip
// as json.Number instead of being coerced to float64, which would silently alter
// the forked specification before it is fingerprinted and sent to Tableau.
func forkCloneMap(value map[string]any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return map[string]any{}
	}
	return result
}
func forkStringValue(value any) string { result, _ := value.(string); return result }
func forkReadFail(input ForkInput, summary string, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Review the exact source metric and selected site, then retry.")
	return &errs.Error{ID: "pulse.metric.fork.read", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted}
}
func forkFail(id string, kind errs.Kind, input ForkInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact source metric and supported fork changes, then review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}

// ValidateInput validates local changes; allowed fields and grains require live evidence.
func forkValidateInput(input *ForkInput) error {
	timeframe := strings.ToUpper(strings.TrimSpace(input.Timeframe))
	if strings.TrimSpace(input.MetricLUID) == "" {
		return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "Pulse metric fork requires an exact source metric LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
		return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "Pulse metric fork requires a well-formed exact source metric LUID.", err)
	}
	if timeframe == "" && len(input.Filters) == 0 {
		return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "Pulse metric fork requires a timeframe or dimensional filter.", nil)
	}
	daysSet := input.CustomDaysSet || input.CustomDays != 0
	if timeframe == "CUSTOM_N_DAYS" && !daysSet {
		return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "--period CUSTOM_N_DAYS requires --days.", nil)
	}
	if daysSet && timeframe != "CUSTOM_N_DAYS" {
		return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "--days requires CUSTOM_N_DAYS.", nil)
	}
	if daysSet && !ForkIsSupportedCustomDays(input.CustomDays) {
		return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "--days must be one of 7, 14, 30, 60, or 90.", nil)
	}
	var period map[string]any
	if timeframe != "" {
		var ok bool
		if period, ok = forkMeasurementPeriod(timeframe, input.CustomDays); !ok {
			return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "Pulse metric fork timeframe is not supported.", nil)
		}
	}
	filters := forkCanonicalFilters(input.Filters)
	for _, filter := range filters {
		if filter.Field == "" || len(filter.Values) == 0 || !forkValidFilterValues(filter.Values) {
			return forkFail("pulse.metric.fork.usage", errs.KindUsage, *input, "Every dimensional filter must name a field and bounded nonempty values.", nil)
		}
	}
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	input.Timeframe, input.Filters, input.period = timeframe, filters, period
	return nil
}

// IsSupportedCustomDays reports whether days is one of Tableau Pulse's bounded
// custom trailing periods.
func ForkIsSupportedCustomDays(days int) bool {
	return slices.Contains(ForkSupportedCustomDays(), days)
}

// SupportedCustomDays returns the accepted custom trailing periods.
func ForkSupportedCustomDays() []int { return []int{7, 14, 30, 60, 90} }
