package create

import "strings"

func compactPlan(plan Plan) CompactPlan {
	request := plan.Request
	dimensions := append([]string{}, plan.Dimensions...)
	omitted := max(0, len(dimensions)-50)
	dimensions = dimensions[:min(50, len(dimensions))]
	granularities := make([]string, len(request.ExtensionOptions.AllowedGranularities))
	for i, value := range request.ExtensionOptions.AllowedGranularities {
		granularities[i] = strings.TrimPrefix(value, "GRANULARITY_BY_")
	}
	minimum := ""
	if len(granularities) > 0 {
		minimum = granularities[0]
	}
	comparisons := []string{}
	for _, value := range request.Comparisons.Comparisons {
		comparisons = append(comparisons, strings.TrimPrefix(value.CompareConfig.Comparison, "TIME_COMPARISON_"))
	}
	disabled := []string{}
	for _, value := range request.InsightsOptions.Settings {
		if value.Disabled {
			disabled = append(disabled, strings.TrimPrefix(value.Type, "INSIGHT_TYPE_"))
		}
	}
	sentiment := strings.TrimPrefix(request.RepresentationOptions.SentimentType, "SENTIMENT_TYPE_")
	sentiment = strings.TrimSuffix(sentiment, "_IS_GOOD")
	temporality := strings.TrimPrefix(request.Specification.Temporality, "TEMPORALITY_")
	if temporality == "LATEST_POINT_IN_TIME" {
		temporality = "LATEST"
	}
	return CompactPlan{Mode: plan.Mode, Operation: plan.Operation, Environment: plan.Environment, Site: plan.Site, Name: plan.Name, Description: request.Description, Datasource: plan.Datasource, Measure: Measure{Field: plan.Measure.Field, Aggregation: strings.TrimPrefix(plan.Measure.Aggregation, "AGGREGATION_")}, TimeField: plan.TimeField, Dimensions: dimensions, DimensionsOmitted: omitted, Population: "ALL_ROWS", MinimumGranularity: minimum, AllowedGranularities: granularities, NumberFormat: strings.TrimPrefix(request.RepresentationOptions.Type, "NUMBER_FORMAT_TYPE_"), Currency: strings.TrimPrefix(request.RepresentationOptions.CurrencyCode, "CURRENCY_CODE_"), Sentiment: sentiment, Temporality: temporality, RunningTotal: request.Specification.RunningTotal, OffsetFromToday: request.ExtensionOptions.OffsetFromToday, UseDynamicOffset: request.ExtensionOptions.UseDynamicOffset, Comparisons: comparisons, InsightsEnabled: request.InsightsOptions.ShowInsights, DisabledInsights: disabled, Certified: request.Certification.IsCertified, RequiresFull: omitted > 0, ReviewComplete: omitted == 0}
}
