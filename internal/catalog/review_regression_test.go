package catalog

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestFullGenerationPreservesEveryListProjection(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	scopes := []string{"users", "groups", "projects", "workbooks", "datasources", "flows"}
	writer, err := store.BeginGeneration(ctx, GenerationMetadata{Environment: "dev", Site: "site", GeneratedAt: now, Source: "tableau-rest", RequestedScopes: scopes})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	rows := map[string][]any{
		"users":       {"user", "User", "", "Viewer", "", `{"site_role":"Viewer","preserved":"user"}`},
		"groups":      {"group", "Group", "local", `{"domain":"local","preserved":"group"}`},
		"projects":    {"project", "Project", "", "Description", "user", `{"description":"Description","preserved":"project"}`},
		"workbooks":   {"workbook", "Workbook", "project", "user", int64(12), "", `{"tags":["daily"],"preserved":"workbook"}`},
		"datasources": {"datasource", "Datasource", "project", "user", "", `{"is_certified":true,"preserved":"datasource"}`},
		"flows":       {"flow", "Flow", "project", "user", "tfl", "", `{"file_type":"tfl","preserved":"flow"}`},
	}
	for _, scope := range scopes {
		if err := writer.WriteBatch(ctx, Batch{Scope: scope, Columns: batchColumns[scope], Rows: [][]any{rows[scope]}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.CompleteScopes(ctx, scopes); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"user", "group", "project", "workbook", "datasource", "flow"} {
		result, err := store.ReadResources(ctx, ResourceQuery{Environment: "dev", Site: "site", Kind: kind, Limit: 1})
		if err != nil || len(result.Entries) != 1 {
			t.Fatalf("%s: %#v %v", kind, result, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(result.Entries[0].Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["preserved"] != kind || payload["luid"] != kind {
			t.Fatalf("%s projection: %#v", kind, payload)
		}
		if kind == "workbook" || kind == "datasource" || kind == "flow" {
			if payload["project_path"] != "Project" {
				t.Fatalf("canonical path: %#v", payload)
			}
		}
	}
}
