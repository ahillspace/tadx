package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFlowRecordCacheShapeAndOperationProjections(t *testing.T) {
	record := Record{
		LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectName: "Ops",
		ProjectPath: "Division/Ops", FileType: "tflx", UpdatedAt: "2026-09-01T00:00:00Z",
		OwnerLUID: "owner-1", Tags: []string{"reviewed"},
		Parameters:  []InspectParameter{{Name: "region"}},
		OutputSteps: []InspectOutputStep{{LUID: "step-1", Name: "Output"}},
		RequestID:   "private-request",
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"luid":"flow-1","name":"Daily","project_luid":"project-1","project_name":"Ops","project_path":"Division/Ops","file_type":"tflx","updated_at":"2026-09-01T00:00:00Z","owner_luid":"owner-1","tags":["reviewed"],"parameters":[{"name":"region"}],"output_steps":[{"luid":"step-1","name":"Output"}]}` {
		t.Fatalf("normalized flow record JSON changed: %s", encoded)
	}
	list, err := json.Marshal(ListOutput{Flows: []Record{record}}.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(list), "parameters") || strings.Contains(string(list), "output_steps") || strings.Contains(string(list), "private-request") {
		t.Fatalf("list projection leaked inspect fields: %s", list)
	}
	pull, err := json.Marshal(PullOutput{Flow: record}.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pull), "owner_luid") || strings.Contains(string(pull), "parameters") || strings.Contains(string(pull), "project_name") {
		t.Fatalf("pull projection leaked record fields: %s", pull)
	}
	if got := contentIdentity(record); got.LUID != "flow-1" || got.ProjectPath != "Division/Ops" {
		t.Fatalf("delete identity = %#v", got)
	}
}
