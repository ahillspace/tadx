package flow_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	flowops "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/cache"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
)

func publishedFlowDetail(t *testing.T, item flowops.Record) cache.ResourceEntry {
	t.Helper()
	store := cache.NewStore(t.TempDir(), time.Now)
	ports := resourceflow.InventoryPorts{Store: func() *cache.Store { return store }}
	ports.PublishFlowInspect(t.Context(), flowops.InspectOutput{Environment: "dev", Site: "sandbox", Flow: item}, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC))
	result, err := store.ReadResources(t.Context(), cache.ResourceQuery{Environment: "dev", Site: "sandbox", Kind: "flow", LUID: item.LUID, Limit: 1, ExactlyOne: true})
	if err != nil || len(result.Entries) != 1 {
		t.Fatalf("cached result=%#v err=%v", result, err)
	}
	return result.Entries[0]
}

func TestFlowInspectCachePayloadPreservesOriginalProjection(t *testing.T) {
	item := flowops.Record{
		LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectName: "Ops",
		FileType: "tflx", OwnerLUID: "owner-1", Tags: []string{"daily"},
		Parameters:  []flowops.InspectParameter{{Name: "region", Required: new(false)}},
		OutputSteps: []flowops.InspectOutputStep{{LUID: "step-1", Name: "Output"}},
		RequestID:   "private-request",
	}
	entry := publishedFlowDetail(t, item)
	want := `{"luid":"flow-1","name":"Daily","project_luid":"project-1","project_path":"","file_type":"tflx","owner_luid":"owner-1","tags":["daily"],"parameters":[{"name":"region","required":false}],"output_steps":[{"luid":"step-1","name":"Output"}]}`
	if string(entry.Payload) != want || entry.ProjectLUID != "project-1" {
		t.Fatalf("entry=%+v payload=%s", entry, entry.Payload)
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
	entry := publishedFlowDetail(t, item)
	var cached flowops.Record
	if err := json.Unmarshal(entry.Payload, &cached); err != nil {
		t.Fatal(err)
	}
	if len(cached.Tags) != 51 || strings.Contains(string(entry.Payload), "tags_omitted") {
		t.Fatalf("detail cache truncated tags: %s", entry.Payload)
	}
}
