package value_test

import (
	"encoding/json"
	"reflect"
	"testing"

	datasourcedelete "github.com/ahillspace/tadx/actions/datasource/delete"
	datasourcemove "github.com/ahillspace/tadx/actions/datasource/move"
	datasourceschema "github.com/ahillspace/tadx/actions/datasource/schema"
	datasourceupdate "github.com/ahillspace/tadx/actions/datasource/update"
	flowdelete "github.com/ahillspace/tadx/actions/flow/delete"
	flowmove "github.com/ahillspace/tadx/actions/flow/move"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	workbookdelete "github.com/ahillspace/tadx/actions/workbook/delete"
	workbookmove "github.com/ahillspace/tadx/actions/workbook/move"
	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
	"github.com/ahillspace/tadx/internal/tableau/metadata"
)

func TestIdenticalValuesShareOneType(t *testing.T) {
	for name, values := range map[string][]any{
		"content identity":       {workbookdelete.Workbook{}, datasourcedelete.Datasource{}, flowdelete.Flow{}, flowmove.Flow{}},
		"owned content identity": {workbookmove.Workbook{}, datasourcemove.Datasource{}, datasourceupdate.Datasource{}},
		"project identity":       {workbookmove.Project{}, datasourcemove.Project{}, flowmove.Project{}},
		"schema field":           {datasourceschema.Field{}, fieldcatalog.Field{}},
		"schema table":           {datasourceschema.Table{}, fieldcatalog.Table{}},
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
	if reflect.TypeOf(workbookdelete.Workbook{}) == reflect.TypeOf(workbookmove.Workbook{}) {
		t.Fatal("smaller content identity was widened to an owner-bearing identity")
	}
	// Workbook updates now need description evidence; movement identities do not.
	if _, ok := reflect.TypeOf(workbookupdate.Workbook{}).FieldByName("Description"); !ok {
		t.Fatal("workbook updates lost description drift evidence")
	}
}

func TestSharedValuesPreserveActionJSON(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"content", workbookdelete.Workbook{}, `{"luid":"","name":"","project_luid":"","project_path":""}`},
		{"owned content", workbookmove.Workbook{}, `{"luid":"","name":"","project_luid":"","project_path":"","owner_luid":""}`},
		{"project", flowmove.Project{}, `{"luid":"","name":"","path":""}`},
		{"field", datasourceschema.Field{}, `{"id":"","name":"","caption":"","label":"","role":"","data_type":"","requires_user_aggregation":false,"excluded":false}`},
		{"table", datasourceschema.Table{}, `{"id":"","name":"","field_count":0}`},
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
