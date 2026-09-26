package datasource

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResourceRecordUsesSeparateOperationProjections(t *testing.T) {
	item := Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops", Tags: []string{"daily"}, Upstream: &Upstream{Status: "observed"}, RequestID: "private-request"}
	for _, test := range []struct {
		name       string
		projection any
		want       string
	}{
		{"inspect", inspectRecord(item), `"upstream"`},
		{"list", listRecord(item), `"project_name":"Ops"`},
		{"pull", pullIdentity(item), `"project_luid":"p-1"`},
		{"delete", deleteIdentity(item), `"project_path":""`},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, err := json.Marshal(test.projection)
			if err != nil || !strings.Contains(string(b), test.want) || strings.Contains(string(b), "private-request") {
				t.Fatalf("projection=%s err=%v", b, err)
			}
			if test.name != "inspect" && strings.Contains(string(b), "upstream") {
				t.Fatalf("metadata leaked: %s", b)
			}
			if test.name != "list" && strings.Contains(string(b), "project_name") {
				t.Fatalf("project name leaked: %s", b)
			}
		})
	}
	inspect, list := inspectRecord(item), listRecord(item)
	inspect.Tags[0], list.Tags[0] = "inspect", "list"
	if item.Tags[0] != "daily" {
		t.Fatal("projection mutated record tags")
	}
}
