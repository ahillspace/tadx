package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	flowops "github.com/ahillspace/tadx/actions/flow"
)

func TestFlowInspectCachePayloadPreservesOriginalProjection(t *testing.T) {
	item := flowops.Record{
		LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectName: "Ops",
		FileType: "tflx", OwnerLUID: "owner-1", Tags: []string{"daily"},
		Parameters:  []flowops.InspectParameter{{Name: "region", Required: new(false)}},
		OutputSteps: []flowops.InspectOutputStep{{LUID: "step-1", Name: "Output"}},
		RequestID:   "private-request",
	}
	entry, err := resourceEntry("dev", "sandbox", "flow", item.LUID, item.Name, item.ProjectPath, item.OwnerLUID, "detail", time.Time{}, (flowops.InspectOutput{Flow: item}).CacheFlow())
	want := `{"luid":"flow-1","name":"Daily","project_luid":"project-1","project_path":"","file_type":"tflx","owner_luid":"owner-1","tags":["daily"],"parameters":[{"name":"region","required":false}],"output_steps":[{"luid":"step-1","name":"Output"}]}`
	if err != nil || string(entry.Payload) != want || entry.ProjectLUID != "project-1" {
		t.Fatalf("entry=%+v payload=%s err=%v", entry, entry.Payload, err)
	}
	var cached flowops.Record
	if err := json.Unmarshal(entry.Payload, &cached); err != nil {
		t.Fatal(err)
	}
	if cached.ProjectName != "" || cached.ProjectPath != "" || cached.RequestID != "" || len(cached.Parameters) != 1 || len(cached.OutputSteps) != 1 {
		t.Fatalf("cached=%+v", cached)
	}
	listed, err := json.Marshal((flowops.ListOutput{Flows: []flowops.Record{cached}}).FullOutput())
	if err != nil || !json.Valid(listed) {
		t.Fatalf("cached list=%s err=%v", listed, err)
	}
	if strings.Contains(string(listed), "project_name") {
		t.Fatalf("cached list gained project name: %s", listed)
	}
}

func TestFlowInspectCachePayloadDoesNotTruncateDetails(t *testing.T) {
	tags := make([]string, 51)
	for index := range tags {
		tags[index] = "tag"
	}
	item := flowops.Record{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", Tags: tags}
	entry, err := resourceEntry("dev", "sandbox", "flow", item.LUID, item.Name, item.ProjectPath, item.OwnerLUID, "detail", time.Time{}, (flowops.InspectOutput{Flow: item}).CacheFlow())
	if err != nil {
		t.Fatal(err)
	}
	var cached flowops.Record
	if err := json.Unmarshal(entry.Payload, &cached); err != nil {
		t.Fatal(err)
	}
	if len(cached.Tags) != 51 || strings.Contains(string(entry.Payload), "tags_omitted") {
		t.Fatalf("detail cache truncated tags: %s", entry.Payload)
	}
}
