package pulse

import (
	"context"
	"strings"
	"testing"

	definition "github.com/ahillspace/tadx/actions/pulse/definition"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

func TestPulseDefinitionFieldValidatorUsesExactRawIDsAndAggregationRules(t *testing.T) {
	schema := fieldcatalog.Schema{
		DatasourceLUID: "datasource-1",
		DatasourceName: "Orders",
		Fields: []fieldcatalog.Field{
			{ID: "[Revenue]", Caption: "Revenue", Role: "measure", DataType: "NUMBER"},
			{ID: "[Margin Ratio]", Caption: "Margin Ratio", Role: "measure", DataType: "NUMBER", RequiresUserAggregation: true},
			{ID: "[Flat Fee]", Caption: "Flat Fee", Role: "measure", DataType: "NUMBER", DefaultAggregation: "SUM"},
			{ID: "[Nested Rank]", Caption: "Nested Rank", Role: "excluded", DataType: "NUMBER", Excluded: true, ExclusionReason: "table_calc"},
			{ID: "[Order Date]", Caption: "Order Date", Role: "date", DataType: "DATE"},
			{ID: "[Region]", Caption: "Region", Role: "dimension", DataType: "STRING"},
			{ID: "[Hidden]", Caption: "Hidden", Role: "excluded", DataType: "STRING", Excluded: true, ExclusionReason: "internal"},
		},
	}
	adapter := resourcedatasource.NewSchemaAdapter(pulseSchemaIdentityStub{}, pulseSchemaStub{schema: schema})
	validator := &DefinitionFieldPort{Schema: adapter}
	fields := make(map[string][]fieldcatalog.Field, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.ID] = append(fields[field.ID], field)
	}

	valid := definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]", AllowedDimensions: []string{"[Region]"}}
	if err := validator.validateFields(fields, valid); err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		input       definition.CreateFieldReferences
		wantMessage string
	}{
		"caption instead of raw ID":      {input: definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "Revenue", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]"}},
		"wrong date role":                {input: definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Region]"}},
		"excluded dimension":             {input: definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]", AllowedDimensions: []string{"[Hidden]"}}},
		"nested table calculation":       {input: definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Nested Rank]", Aggregation: "AGGREGATION_USER", TimeDimension: "[Order Date]"}, wantMessage: "table calculations cannot be used as Pulse measures"},
		"aggregate calculation plus SUM": {input: definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Margin Ratio]", Aggregation: "AGGREGATION_SUM", TimeDimension: "[Order Date]"}, wantMessage: "already aggregated; use --aggregation USER"},
		"unexpected user aggregation":    {input: definition.CreateFieldReferences{DatasourceLUID: schema.DatasourceLUID, MeasureField: "[Revenue]", Aggregation: "AGGREGATION_USER", TimeDimension: "[Order Date]"}},
	} {
		t.Run(name, func(t *testing.T) {
			err := validator.validateFields(fields, test.input)
			if err == nil {
				t.Fatal("expected validation failure")
			}
			if test.wantMessage != "" && !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("error = %q, want substring %q", err, test.wantMessage)
			}
		})
	}
	for name, measure := range map[string]struct {
		field       string
		aggregation string
	}{
		"aggregate calculation plus USER": {field: "[Margin Ratio]", aggregation: "AGGREGATION_USER"},
		"row-level calculation plus SUM":  {field: "[Flat Fee]", aggregation: "AGGREGATION_SUM"},
	} {
		t.Run(name, func(t *testing.T) {
			valid.MeasureField = measure.field
			valid.Aggregation = measure.aggregation
			if err := validator.validateFields(fields, valid); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPulseFieldResolutionAndBundleValidationPreserveCanonicalIdentity(t *testing.T) {
	schema := fieldcatalog.Schema{DatasourceLUID: "datasource-1", DatasourceName: "Orders", Fields: []fieldcatalog.Field{
		{ID: "[count_raw]", Caption: "People", Role: "dimension", DataType: "STRING"},
		{ID: "[date_raw]", Caption: "Order Date", Role: "date", DataType: "DATE"},
		{ID: "[region_raw]", Caption: "Region", Role: "dimension", DataType: "STRING"},
	}}
	v := &DefinitionFieldPort{Schema: resourcedatasource.NewSchemaAdapter(pulseSchemaIdentityStub{}, pulseSchemaStub{schema: schema})}
	refs, err := v.ResolveDefinitionFields(context.Background(), definition.CreateFieldReferences{DatasourceLUID: "datasource-1", MeasureField: "People", Aggregation: "AGGREGATION_COUNT_DISTINCT", TimeDimension: "Order Date", AllowedDimensions: []string{"Region"}})
	if err != nil || refs.MeasureField != "[count_raw]" || refs.TimeDimension != "[date_raw]" || refs.AllowedDimensions[0] != "[region_raw]" {
		t.Fatalf("refs=%#v err=%v", refs, err)
	}
	// A disappeared canonical ID must not be reinterpreted as another field's caption.
	schema.Fields[0].ID = "[replacement]"
	schema.Fields[0].Caption = "[count_raw]"
	fields := make(map[string][]fieldcatalog.Field, len(schema.Fields))
	for _, field := range schema.Fields {
		fields[field.ID] = append(fields[field.ID], field)
	}
	if err := v.validateFields(fields, refs); err == nil {
		t.Fatal("canonical field disappearance silently retargeted to a caption")
	}
}

type pulseSchemaIdentityStub struct{}

func (pulseSchemaIdentityStub) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return tableaudatasource.Datasource{LUID: "datasource-1", Name: "Orders"}, nil
}

type pulseSchemaStub struct{ schema fieldcatalog.Schema }

func (s pulseSchemaStub) Read(context.Context, string, string) (fieldcatalog.Schema, error) {
	return s.schema, nil
}
