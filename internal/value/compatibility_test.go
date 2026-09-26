package value_test

import (
	"encoding/json"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	"reflect"
	"testing"

	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	workbookops "github.com/ahillspace/tadx/actions/workbook"

	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	"github.com/ahillspace/tadx/internal/tableau/metadata"
)

func TestIdenticalValuesShareOneType(t *testing.T) {
	for name, values := range map[string][]any{
		"content identity":       {workbookops.DeletePlan{}.Target, datasourceops.DeletePlan{}.Target, flowops.DeleteFlow{}, flowops.MoveFlow{}},
		"owned content identity": {workbookops.MovePlan{}.Source, datasourceops.MovePlan{}.Source, datasourceops.UpdatePlan{}.Target},
		"project identity":       {workbookops.Project{}, datasourceops.Project{}, flowops.Project{}},
		"schema field":           {datasourceops.Field{}, fieldcatalog.Field{}},
		"schema table":           {datasourceops.Table{}, fieldcatalog.Table{}},
		"lineage node":           {lineagepull.Node{}, resourcelineage.Node{}, metadata.Node{}},
		"lineage edge":           {lineagepull.Edge{}, resourcelineage.Edge{}, metadata.Edge{}},
	} {
		t.Run(name, func(t *testing.T) {
			want := reflect.TypeOf(values[0])
			for _, candidate := range values[1:] {
				if got := reflect.TypeOf(candidate); got != want {
					t.Errorf("identical values retain separate types: %s and %s", want, got)
				}
			}
		})
	}
	if reflect.TypeOf(workbookops.DeletePlan{}.Target) == reflect.TypeOf(workbookops.MovePlan{}.Source) {
		t.Fatal("smaller content identity was widened to an owner-bearing identity")
	}
	// Workbook updates now need description evidence; movement identities do not.
	if _, ok := reflect.TypeOf(workbookops.UpdatePlan{}.Target).FieldByName("Description"); !ok {
		t.Fatal("workbook updates lost description drift evidence")
	}
}

func TestSharedValuesPreserveActionJSON(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"content", workbookops.DeletePlan{}.Target, `{"luid":"","name":"","project_luid":"","project_path":""}`},
		{"owned content", workbookops.MovePlan{}.Source, `{"luid":"","name":"","project_luid":"","project_path":"","owner_luid":""}`},
		{"project", flowops.Project{}, `{"luid":"","name":"","path":""}`},
		{"field", datasourceops.Field{}, `{"id":"","name":"","caption":"","label":"","role":"","data_type":"","requires_user_aggregation":false,"excluded":false}`},
		{"table", datasourceops.Table{}, `{"id":"","name":"","field_count":0}`},
		{"node", lineagepull.Node{}, `{"metadata_id":"","kind":""}`},
		{"edge", lineagepull.Edge{}, `{"from_metadata_id":"","to_metadata_id":"","relationship":""}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := json.Marshal(test.value)
			if err != nil || string(got) != test.want {
				t.Fatalf("json=%s err=%v; want %s", got, err, test.want)
			}
		})
	}
}
