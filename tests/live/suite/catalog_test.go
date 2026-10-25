package suite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestCatalogReadRejectsEmptyAndIncorrectLiveOutput(t *testing.T) {
	binary := contentCandidate(t)
	for _, mutation := range []string{"correct", "empty", "wrong-description", "wrong-metadata-id"} {
		t.Run(mutation, func(t *testing.T) {
			restReads, metadataReads := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/databases/database-a"):
					restReads++
					_ = json.NewEncoder(w).Encode(map[string]any{"database": map[string]any{"id": "database-a", "name": "Sales fixture", "description": "Authored description"}})
				case r.Method == "POST" && r.URL.Path == "/api/metadata/graphql":
					metadataReads++
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"databases": []any{map[string]any{"id": "metadata-a", "name": "Sales fixture"}}}})
				default:
					t.Errorf("unexpected independent request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			row := map[string]any{"luid": "database-a", "metadata_id": "metadata-a", "name": "Sales fixture", "description": "Authored description"}
			switch mutation {
			case "wrong-description":
				row["description"] = "Fabricated description"
			case "wrong-metadata-id":
				row["metadata_id"] = "metadata-decoy"
			}
			items := []any{row}
			if mutation == "empty" {
				items = []any{}
			}
			data, _ := json.Marshal(map[string]any{"items": items})
			c := testAdminCase(t, server, binary, string(data))
			c.Profile.Bindings = map[string]harness.Binding{"catalog_database": {ID: "database-a", Name: "Sales fixture", Params: map[string]string{"metadata_id": "metadata-a", "oracle_query": "query { databases { id name } }"}, Expected: map[string]any{"oracle": map[string]any{"databases": []any{map[string]any{"id": "metadata-a", "name": "Sales fixture"}}}}}}
			err := catalogRead("database")(t.Context(), c)
			if (err == nil) != (mutation == "correct") {
				t.Fatalf("mutation %s: %v", mutation, err)
			}
			if restReads != 1 {
				t.Fatalf("REST oracle reads=%d", restReads)
			}
			if mutation == "correct" && metadataReads != 1 {
				t.Fatal("successful read skipped independent Metadata oracle")
			}
		})
	}
}

func TestAuthoredTruthRequiresExactArrayCoverage(t *testing.T) {
	want := []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}
	for _, actual := range []any{[]any{}, []any{map[string]any{"id": "a"}, map[string]any{"id": "a"}}, []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}, map[string]any{"id": "decoy"}}} {
		if subset(actual, want) == nil {
			t.Fatalf("incomplete or duplicate actual set passed: %v", actual)
		}
	}
	if err := subset([]any{map[string]any{"id": "b", "name": "B"}, map[string]any{"id": "a", "name": "A"}}, want); err != nil {
		t.Fatal(err)
	}
}
