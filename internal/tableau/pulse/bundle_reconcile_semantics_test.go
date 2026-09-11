package pulse_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
)

const bundleSemanticBase = `{"name":"Revenue","description":"Portable","specification":{"datasource":{"id":"datasource-1"},"basic_specification":{"measure":{"field":"Revenue","aggregation":"AGGREGATION_SUM"},"time_dimension":{"field":"Date"},"filters":[]}}`

func TestBundleDefinitionReadbackNormalizesKnownComparisonIndexes(t *testing.T) {
	for _, shape := range []string{"list", "root nested", "entry nested", "recursive nested"} {
		for _, pair := range [][2]string{{"1", `"1"`}, {`"1"`, "1"}} {
			t.Run(shape+"/"+pair[0]+"_to_"+pair[1], func(t *testing.T) {
				assertBundleSemanticReadback(t, bundleComparisonDocument(shape, pair[0]), bundleComparisonDocument(shape, pair[1]), "")
			})
		}
	}
	for _, pair := range [][2]string{
		{"9223372036854775807", `"9223372036854775807"`},
		{`"9223372036854775807"`, "9223372036854775807"},
		{"-9223372036854775808", `"-9223372036854775808"`},
		{`"-9223372036854775808"`, "-9223372036854775808"},
		{"9007199254740993", `"9007199254740993"`},
		{`"9007199254740993"`, "9007199254740993"},
		{"-0", `"0"`},
	} {
		t.Run(pair[0]+"_to_"+pair[1], func(t *testing.T) {
			assertBundleSemanticReadback(t, bundleComparisonDocument("list", pair[0]), bundleComparisonDocument("list", pair[1]), "")
		})
	}
}

func TestBundleDefinitionReadbackRejectsInvalidObservedComparisonIndexes(t *testing.T) {
	for _, value := range []string{`"bad"`, `"9223372036854775808"`, `"-9223372036854775809"`, "9223372036854775808", "-9223372036854775809", "true", "null", "1.5", "1e0", `"1.0"`, `"+1"`, `"01"`} {
		t.Run(value, func(t *testing.T) {
			assertBundleSemanticReadback(t, bundleComparisonDocument("list", "1"), bundleComparisonDocument("list", value), "saved definition configuration: comparisons.comparisons[0].index")
		})
	}
	for _, shape := range []string{"root nested", "entry nested", "recursive nested"} {
		t.Run(shape, func(t *testing.T) {
			assertBundleSemanticReadback(t, bundleComparisonDocument(shape, "1"), bundleComparisonDocument(shape, `"bad"`), "saved definition configuration: comparisons.")
		})
	}
}

func TestBundleDefinitionReadbackPreservesComparisonDifferencesAndOrder(t *testing.T) {
	for _, pair := range [][2]string{{"1", `"2"`}, {`"9007199254740993"`, `"9007199254740992"`}, {"9007199254740993", "9007199254740992"}} {
		t.Run(pair[0]+"_versus_"+pair[1], func(t *testing.T) {
			assertBundleSemanticReadback(t, bundleComparisonDocument("list", pair[0]), bundleComparisonDocument("list", pair[1]), `configuration mismatch in "comparisons"`)
		})
	}
	for _, tc := range []struct{ name, submitted, saved string }{
		{"unknown nested field", `{"comparisons":[{"index":1,"extension":{"index":1}}]}`, `{"comparisons":[{"index":"1","extension":{"index":"1"}}]}`},
		{"comparison order", `{"comparisons":[{"index":1},{"index":2}]}`, `{"comparisons":[{"index":"2"},{"index":"1"}]}`},
		{"missing index", `{"comparisons":[{"index":1}]}`, `{"comparisons":[{}]}`},
		{"comparison business field", `{"comparisons":[{"index":1,"compare_config":{"comparison":"TIME_COMPARISON_YEAR_AGO_PERIOD"}}]}`, `{"comparisons":[{"index":"1","compare_config":{"comparison":"TIME_COMPARISON_NONE"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertBundleSemanticReadback(t, bundleSemanticBase+`,"comparisons":`+tc.submitted+`}`, bundleSemanticBase+`,"comparisons":`+tc.saved+`}`, `configuration mismatch in "comparisons"`)
		})
	}
}

func TestBundleDefinitionReadbackNormalizesBothGoalFilterSpecifications(t *testing.T) {
	for _, specification := range []string{"basic_specification", "threshold_basic_specification"} {
		for _, tc := range []struct {
			name                          string
			reverseValues, reverseFilters bool
		}{
			{name: "identical"},
			{name: "categorical value order", reverseValues: true},
			{name: "filter order", reverseFilters: true},
			{name: "both orders", reverseValues: true, reverseFilters: true},
		} {
			t.Run(specification+"/"+tc.name, func(t *testing.T) {
				assertBundleSemanticReadback(t, bundleGoalDocument(specification, false, false), bundleGoalDocument(specification, tc.reverseValues, tc.reverseFilters), "")
			})
		}
	}
}

func TestBundleDefinitionReadbackPreservesGoalFilterDifferencesAndGoalOrder(t *testing.T) {
	for _, specification := range []string{"basic_specification", "threshold_basic_specification"} {
		for _, change := range [][2]string{
			{`"West"`, `"North"`},
			{`"OPERATOR_EQUAL"`, `"OPERATOR_NOT_EQUAL"`},
			{`"include_null":true`, `"include_null":false`},
			{`"field":"Region"`, `"field":"Country"`},
		} {
			t.Run(specification+"/"+change[0], func(t *testing.T) {
				submitted := bundleGoalDocument(specification, false, false)
				saved := strings.Replace(bundleGoalDocument(specification, true, true), change[0], change[1], 1)
				assertBundleSemanticReadback(t, submitted, saved, `configuration mismatch in "datasource_goals"`)
			})
		}
	}
	assertBundleSemanticReadback(t,
		bundleSemanticBase+`,"datasource_goals":[{"name":"Primary"},{"name":"Secondary"}]}`,
		bundleSemanticBase+`,"datasource_goals":[{"name":"Secondary"},{"name":"Primary"}]}`,
		`configuration mismatch in "datasource_goals"`)
}

func bundleComparisonDocument(shape, index string) string {
	entry := fmt.Sprintf(`{"index":%s,"compare_config":{"comparison":"TIME_COMPARISON_YEAR_AGO_PERIOD"}}`, index)
	var section string
	switch shape {
	case "list":
		section = `{"comparisons":[` + entry + `]}`
	case "root nested":
		section = `{"nestedComparison":` + entry + `}`
	case "entry nested":
		section = `{"comparisons":[{"nestedComparison":` + entry + `}]}`
	case "recursive nested":
		section = `{"nestedComparison":{"nestedComparison":{"nestedComparison":` + entry + `}}}`
	default:
		panic("unsupported comparison fixture shape")
	}
	return bundleSemanticBase + `,"comparisons":` + section + `}`
}

func bundleGoalDocument(specification string, reverseValues, reverseFilters bool) string {
	values := `[{"string_value":"West"},{"string_value":"East"}]`
	if reverseValues {
		values = `[{"string_value":"East"},{"string_value":"West"}]`
	}
	region := `{"field":"Region","operator":"OPERATOR_EQUAL","categorical_values":` + values + `,"include_null":true}`
	segment := `{"field":"Segment","operator":"OPERATOR_NOT_EQUAL","categorical_values":[{"string_value":"Consumer"}],"include_null":false}`
	filters := region + "," + segment
	if reverseFilters {
		filters = segment + "," + region
	}
	return bundleSemanticBase + `,"datasource_goals":[{"name":"Target","` + specification + `":{"filters":[` + filters + `]}}]}`
}

func assertBundleSemanticReadback(t *testing.T, submitted, saved, wantError string) {
	t.Helper()
	readback := strings.Replace(saved, `"name":"Revenue","description":"Portable"`, `"metadata":{"id":"definition-1","name":"Revenue","description":"Portable"}`, 1)
	reads := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads++
		if r.Method != http.MethodGet || r.URL.Path != "/api/-/pulse/definitions/definition-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, readback)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	raw := json.RawMessage(submitted)
	err := client.VerifyBundleDefinition(context.Background(), "definition-1", "datasource-1", "site-1", raw)
	if (wantError == "" && err != nil) || (wantError != "" && (err == nil || !strings.Contains(err.Error(), wantError))) {
		t.Errorf("verification error=%v, want %q", err, wantError)
	}
	if reads != 1 {
		t.Errorf("definition reads=%d, want 1", reads)
	}
	if !bytes.Equal(raw, []byte(submitted)) {
		t.Error("verification changed submitted payload bytes")
	}
}
