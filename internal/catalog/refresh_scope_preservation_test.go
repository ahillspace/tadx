package catalog

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestRefreshPreservesIndependentResourceKindsAndFreshness(t *testing.T) {
	ctx := context.Background()
	old := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return old.Add(48 * time.Hour) })
	kinds := []string{"datasource_schema", "pulse_definition", "pulse_metric", "pulse_follower"}
	before := map[string]ResourceResult{}
	for _, kind := range kinds {
		if err := store.UpsertResources(ctx, []ResourceEntry{{Environment: "dev", Site: "site", Kind: kind, LUID: "id", Name: "Saved", Payload: []byte(`{"saved":true}`), Coverage: "detail", ObservedAt: old}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ReplaceResourceScope(ctx, ResourceScopeReplacement{Environment: "dev", Site: "site", Kind: "workbook", GeneratedAt: old, Source: "tableau-rest", Entries: []ResourceEntry{{LUID: "workbook", Name: "Workbook", ProjectLUID: "project", Coverage: "summary", ObservedAt: old}}}); err != nil {
		t.Fatal(err)
	}
	kinds = append(kinds, "workbook")
	for _, kind := range kinds {
		result, err := store.ReadResources(ctx, ResourceQuery{Environment: "dev", Site: "site", Kind: kind, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		before[kind] = result
	}
	writer, err := store.BeginRefreshGeneration(ctx, GenerationMetadata{Environment: "dev", Site: "site", GeneratedAt: old.Add(48 * time.Hour), Source: "tableau-rest", RequestedScopes: []string{"projects"}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.WriteBatch(ctx, Batch{Scope: "projects", Columns: batchColumns["projects"], Rows: [][]any{{"project", "Project", "", "", "", `{}`}}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.CompleteScopes(ctx, []string{"projects"}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	for _, kind := range kinds {
		result, err := store.ReadResources(ctx, ResourceQuery{Environment: "dev", Site: "site", Kind: kind, Limit: 10})
		if err != nil {
			t.Errorf("%s disappeared: %v", kind, err)
			continue
		}
		if !reflect.DeepEqual(result, before[kind]) {
			t.Errorf("%s changed freshness or data: before %#v after %#v", kind, before[kind], result)
		}
	}
}
