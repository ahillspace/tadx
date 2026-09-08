package fork

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type Reader interface {
	GetMetric(context.Context, string) (Metric, error)
	GetDefinition(context.Context, string) (Definition, error)
}
type Creator interface {
	GetOrCreateMetric(context.Context, CreateRequest) (CreateResult, error)
}
type Reconciler interface {
	ReconcileMetric(context.Context, ExpectedMetric) (Reconciliation, error)
}
type Action struct {
	reader     Reader
	creator    Creator
	reconciler Reconciler
}

func New(reader Reader, creator Creator, reconciler Reconciler) *Action {
	return &Action{reader: reader, creator: creator, reconciler: reconciler}
}
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.reader == nil || a.creator == nil || a.reconciler == nil {
		return Output{}, fail("pulse.metric.fork.unconfigured", errs.KindRuntime, input, "Pulse metric fork is not configured.", nil)
	}
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	input.Timeframe = strings.TrimSpace(strings.ToUpper(input.Timeframe))
	if input.MetricLUID == "" {
		return Output{}, fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires an exact source metric LUID.", nil)
	}
	if input.Timeframe == "" && len(input.Filters) == 0 {
		return Output{}, fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork requires a timeframe or dimensional filter.", nil)
	}
	plan, err := a.plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	output := Output{Plan: plan, Help: []string{"Run without --preview to get or create this exact Pulse metric variant."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	if current.Fingerprint != plan.Fingerprint {
		return Output{}, fail("pulse.metric.fork.changed", errs.KindOperation, input, "The source Pulse metric changed during revalidation.", errors.New("planned specification fingerprint changed"))
	}
	created, err := a.creator.GetOrCreateMetric(ctx, CreateRequest{DefinitionLUID: plan.DefinitionLUID, Specification: cloneMap(plan.Specification)})
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect the remote get-or-create outcome before retrying.")
		return Output{}, &errs.Error{ID: "pulse.metric.fork.failed", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric fork failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if created.MetricLUID == "" {
		return Output{}, fail("pulse.metric.fork.invalid_response", errs.KindOperation, input, "Tableau returned no metric identity for the fork.", nil)
	}
	reconciled, err := a.reconciler.ReconcileMetric(ctx, ExpectedMetric{MetricLUID: created.MetricLUID, DefinitionLUID: plan.DefinitionLUID, DatasourceLUID: plan.DatasourceLUID, SiteLUID: input.SiteLUID})
	if err != nil {
		return Output{}, &errs.Error{ID: "pulse.metric.fork.reconcile", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: created.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric reconciliation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the created metric by exact LUID before retrying.", TableauRequestID: errs.TableauRequestID(err)}
	}
	if reconciled.Status == "ownership_mismatch" || reconciled.Status == "failed" {
		return Output{}, fail("pulse.metric.fork.reconcile", errs.KindOperation, input, "The forked Pulse metric failed ownership reconciliation.", fmt.Errorf("reconciliation status %s", reconciled.Status))
	}
	status := "existing"
	if created.Created {
		status = "created"
	}
	output.Result = &Result{Status: status, MetricLUID: created.MetricLUID, MetricName: created.MetricName, Created: created.Created, ReconciliationStatus: reconciled.Status, ReconciliationAttempts: reconciled.Attempts, OwnershipVerified: reconciled.OwnershipVerified, InventoryVisible: reconciled.InventoryVisible, RequestID: created.RequestID, ReconciliationRequestID: reconciled.RequestID}
	output.Help = []string{"tadx pulse metric inspect --id " + created.MetricLUID}
	return output, nil
}
func (a *Action) plan(ctx context.Context, input Input) (Plan, error) {
	metric, err := a.reader.GetMetric(ctx, input.MetricLUID)
	if err != nil {
		return Plan{}, readFail(input, "Pulse source metric retrieval failed.", err)
	}
	if metric.LUID != input.MetricLUID || metric.DefinitionLUID == "" || metric.Specification == nil {
		return Plan{}, fail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "Tableau returned an incomplete or mismatched source metric.", nil)
	}
	if input.SiteLUID != "" && metric.SiteLUID != "" && metric.SiteLUID != input.SiteLUID {
		return Plan{}, fail("pulse.metric.fork.ownership", errs.KindOperation, input, "The source metric belongs to a different Tableau site.", nil)
	}
	definition, err := a.reader.GetDefinition(ctx, metric.DefinitionLUID)
	if err != nil {
		return Plan{}, readFail(input, "Pulse source definition retrieval failed.", err)
	}
	if definition.LUID != metric.DefinitionLUID || definition.DatasourceLUID == "" {
		return Plan{}, fail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "Tableau returned an incomplete or mismatched source definition.", nil)
	}
	spec := cloneMap(metric.Specification)
	delete(spec, "datasource")
	if input.Timeframe != "" {
		period, ok := measurementPeriod(input.Timeframe, input.CustomDays)
		if !ok {
			return Plan{}, fail("pulse.metric.fork.usage", errs.KindUsage, input, "Pulse metric fork timeframe is not supported.", nil)
		}
		spec["measurement_period"] = period
	}
	if len(definition.AllowedGranularities) == 0 {
		return Plan{}, fail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "The source definition omitted allowed granularities; its supported periods cannot be validated.", nil)
	}
	period, ok := spec["measurement_period"].(map[string]any)
	if !ok || stringValue(period["granularity"]) == "" {
		return Plan{}, fail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "The source metric omitted its period granularity; provide --period to select a supported period.", nil)
	}
	granularity := stringValue(period["granularity"])
	if !slices.Contains(definition.AllowedGranularities, granularity) {
		return Plan{}, fail("pulse.metric.fork.usage", errs.KindUsage, input, "The metric period granularity is not allowed by its definition.", fmt.Errorf("granularity %s is not supported; allowed granularities: %s", granularity, strings.Join(definition.AllowedGranularities, ", ")))
	}
	allowed := map[string]bool{}
	for _, field := range definition.AllowedDimensions {
		allowed[field] = true
	}
	filters := canonicalFilters(input.Filters)
	for _, filter := range filters {
		if filter.Field == "" || len(filter.Values) == 0 || !allowed[filter.Field] || !validFilterValues(filter.Values) {
			return Plan{}, fail("pulse.metric.fork.usage", errs.KindUsage, input, "Every dimensional filter must name an allowed field and at least one value.", nil)
		}
		spec, err = mergeFilter(spec, filter)
		if err != nil {
			return Plan{}, fail("pulse.metric.fork.invalid_source", errs.KindOperation, input, "The source Pulse metric has an unsupported filter specification.", err)
		}
	}
	data, _ := json.Marshal(struct {
		Definition    string         `json:"definition"`
		Specification map[string]any `json:"specification"`
	}{definition.LUID, spec})
	sum := sha256.Sum256(data)
	return Plan{Mode: "preview", Operation: "pulse.metric.fork", Environment: input.Environment, Site: input.Site, SourceMetricLUID: input.MetricLUID, DefinitionLUID: definition.LUID, DatasourceLUID: definition.DatasourceLUID, Timeframe: input.Timeframe, Filters: filters, Specification: spec, Fingerprint: "sha256:" + hex.EncodeToString(sum[:])}, nil
}
func measurementPeriod(key string, days int) (map[string]any, bool) {
	simple := map[string][2]string{"TODAY": {"GRANULARITY_BY_DAY", "RANGE_CURRENT_PARTIAL"}, "THIS_WEEK": {"GRANULARITY_BY_WEEK", "RANGE_CURRENT_PARTIAL"}, "MONTH_TO_DATE": {"GRANULARITY_BY_MONTH", "RANGE_CURRENT_PARTIAL"}, "QUARTER_TO_DATE": {"GRANULARITY_BY_QUARTER", "RANGE_CURRENT_PARTIAL"}, "YEAR_TO_DATE": {"GRANULARITY_BY_YEAR", "RANGE_CURRENT_PARTIAL"}, "YESTERDAY": {"GRANULARITY_BY_DAY", "RANGE_LAST_COMPLETE"}, "LAST_WEEK": {"GRANULARITY_BY_WEEK", "RANGE_LAST_COMPLETE"}, "LAST_MONTH": {"GRANULARITY_BY_MONTH", "RANGE_LAST_COMPLETE"}, "LAST_QUARTER": {"GRANULARITY_BY_QUARTER", "RANGE_LAST_COMPLETE"}, "LAST_YEAR": {"GRANULARITY_BY_YEAR", "RANGE_LAST_COMPLETE"}}
	if pair, ok := simple[key]; ok {
		return map[string]any{"granularity": pair[0], "range": pair[1]}, true
	}
	periods := map[string]int{"LAST_7_DAYS": 7, "LAST_14_DAYS": 14, "LAST_30_DAYS": 30, "LAST_60_DAYS": 60, "LAST_90_DAYS": 90}
	period, ok := periods[key]
	if key == "CUSTOM_N_DAYS" {
		period = days
		ok = days >= 1 && days <= 3650
	}
	if !ok {
		return nil, false
	}
	return map[string]any{"granularity": "GRANULARITY_BY_DAY", "range": "RANGE_BY_CONFIG", "last_x_period": map[string]any{"period": period, "period_type": "GRANULARITY_BY_DAY", "include_current_period": true}}, true
}
func canonicalFilters(values []Filter) []Filter {
	out := make([]Filter, 0, len(values))
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
		sort.Strings(clean)
		value.Values = clean
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}
func mergeFilter(spec map[string]any, filter Filter) (map[string]any, error) {
	raw, ok := spec["filters"].([]any)
	if !ok && spec["filters"] != nil {
		return nil, errors.New("source metric filters must be an array")
	}
	items := make([]any, 0, len(raw)+1)
	matches := 0
	for _, entry := range raw {
		record, ok := entry.(map[string]any)
		if ok && stringValue(record["field"]) == filter.Field {
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

func validFilterValues(values []string) bool {
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
func cloneMap(value map[string]any) map[string]any {
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
func stringValue(value any) string { result, _ := value.(string); return result }
func readFail(input Input, summary string, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Review the exact source metric and selected site, then retry.")
	return &errs.Error{ID: "pulse.metric.fork.read", Kind: errs.KindOperation, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause)}
}
func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.fork", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact source metric and supported fork changes, then review a new preview."}
}
