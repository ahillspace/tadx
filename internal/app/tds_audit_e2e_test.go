package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func TestTDSAuditRetainsHistoricalSourceAndBoundedExport(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	h.fixture.mu.Lock()
	for i := range 25 {
		name := fmt.Sprintf("Measure %02d", i)
		h.fixture.source[tdsEntryKey("Column", name)] = value.TDSElement{Name: name, Column: &value.TDSColumn{Caption: new(name)}}
	}
	h.fixture.mu.Unlock()
	receipt := h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "first.md", "--changes-file", "edits.yaml")
	var checked struct {
		WorkCopyLUID    string                   `json:"work_copy_luid"`
		Checkout        value.TDSCheckoutContext `json:"checkout_receipt"`
		MetadataWarning string                   `json:"metadata_warning"`
		Files           struct {
			Audit   string `json:"audit_file"`
			Changes string `json:"changes_file"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(receipt), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.WorkCopyLUID != fixtureTDSCopy || checked.Checkout.SourceName != "Source" || checked.Checkout.SourceProjectLUID != fixtureTDSProject || checked.MetadataWarning == "" || checked.Files.Audit != "first.md" || checked.Files.Changes != "edits.yaml" {
		t.Fatal("incomplete checkout receipt", receipt)
	}
	for _, when := range []string{checked.Checkout.SourceObservedAt, checked.Checkout.AcknowledgedAt} {
		if _, err := time.Parse(time.RFC3339Nano, when); err != nil {
			t.Fatal(err)
		}
	}
	cmd := h.process("content", "datasource", "work-copy", "--work-copy", fixtureTDSCopy, "--workspace", "work", "--audit-file", "second.md", "--json")
	compact, err := cmd.Output()
	if err != nil {
		t.Fatalf("reinspect: %v: %s", err, compact)
	}
	var inspected struct {
		Checkout     value.TDSCheckoutContext `json:"checkout_receipt"`
		Observations []struct {
			Elements       []value.TDSElement `json:"elements"`
			DisplayOmitted int                `json:"display_omitted"`
		} `json:"observations"`
		Details string `json:"details"`
	}
	if err = json.Unmarshal(compact, &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.Checkout != checked.Checkout || len(inspected.Observations) != 3 || len(inspected.Observations[0].Elements) != 10 || inspected.Observations[0].DisplayOmitted != 16 || !strings.Contains(inspected.Details, "--full") {
		t.Fatalf("unexpected compact re-export: %s", compact)
	}
	for _, file := range []string{"first.md", "second.md"} {
		data, err := os.ReadFile(filepath.Join(h.ws, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Source at checkout", "Source", fixtureTDSProject, checked.Checkout.AcknowledgedAt, "Measure 24", "Observed field-description candidates", "Published datasource Metadata API coverage"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q", file, want)
			}
		}
	}
	// A different PAT for the same Tableau caller can encounter a historical
	// source name which now coincides with its credential. Do not disclose it.
	t.Setenv("TADX_FIXTURE_SECRET", "Source")
	for _, args := range [][]string{{"work-copy", "--attempts"}, {"work-copy", "--work-copy", fixtureTDSCopy}} {
		denied := h.ds(false, args...)
		if strings.Contains(denied, "Source") || !strings.Contains(denied, "protected credential material") {
			t.Fatalf("unsafe historical result %s", denied)
		}
	}
	if h.fixture.writeCount() != 1 {
		t.Fatal("re-export created another copy")
	}
}

func TestTDSAuditReadsOnlyIndexedSourceFieldsInSameSession(t *testing.T) {
	for _, mode := range []string{"complete", "partial", "unavailable", "denied", "schema-only"} {
		t.Run(mode, func(t *testing.T) {
			h := newTDSWorkflowCLI(t)
			h.options = withSiteMutationConsent(t, h.options, true)
			if mode == "denied" {
				delete(h.options.managedPolicy.(fixtureManagedPolicy).allowed, "datasource.schema")
			}
			if mode == "schema-only" {
				delete(h.options.managedPolicy.(fixtureManagedPolicy).allowed, "lineage.pull")
			}
			signIns, graph, vdscalls := 0, 0, 0
			h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
				if strings.HasSuffix(r.URL.Path, "/auth/signin") {
					signIns++
					return false
				}
				if strings.Contains(r.URL.Path, "vizql") {
					vdscalls++
					return false
				}
				if !strings.Contains(r.URL.Path, "metadata") {
					return false
				}
				graph++
				var in struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
					t.Error(err)
				}
				if r.Header.Get("X-Tableau-Auth") != "secret-token" || in.Variables["parent"] != fixtureTDSOriginal || !strings.Contains(in.Query, "fieldsConnection") {
					t.Error("invalid source enrichment session, selector or query")
				}
				relations := strings.Contains(in.Query, "referencedFields:fieldsConnection")
				if !strings.Contains(in.Query, "formula") || relations != (mode != "schema-only") || strings.Contains(in.Query, "downstreamSheetsConnection") != relations {
					t.Errorf("audit requested the wrong calculated-field or lineage projection")
				}
				extras := ""
				if strings.Contains(in.Query, "formula") {
					extras += `"formula":"[Revenue] - [Cost]","__typename":"CalculatedField","role":"MEASURE",`
				}
				if relations {
					extras += `"referencedFields":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"ColumnField","id":"cost-id","name":"Unit cost"}]},"relatedSheets":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"Sheet","id":"sheet-id","name":"Profit overview"}]},`
				}
				// The second page fails. The successfully identified first field remains useful.
				if mode == "unavailable" || graph > 1 {
					w.WriteHeader(403)
					io.WriteString(w, `{"error":{"code":"403004","summary":"Unavailable"}}`)
					return true
				}
				more := mode == "partial"
				total := 1
				if more {
					total++
				}
				fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{
 "id":"source-graph","luid":%q,"name":"Indexed source","description":"Protected: test-secret",
 "tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},
 "items":{"totalCount":%d,"pageInfo":{"hasNextPage":%t,"endCursor":"more"},"nodes":[{
   "id":"index-only","name":"Index only","fullyQualifiedName":"[Only in the index]",
   %s"description":"Session: secret-token","descriptionInherited":[],
   "upstreamColumnsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{
     "id":"upstream-only","luid":"column-id","name":"Physical only","description":"PAT: test-name"
   }]}
 }]}
}]}}}`, fixtureTDSOriginal, total, more, extras)
				return true
			}
			h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "audit.md", "--changes-file", "draft.yaml")
			h.fixture.mu.Lock()
			actualSignIns, actualGraph, actualVDS := signIns, graph, vdscalls
			h.fixture.mu.Unlock()
			expected := map[string]int{"complete": 1, "schema-only": 1, "denied": 0, "unavailable": 1, "partial": 2}[mode]
			if actualSignIns != 1 || actualGraph != expected || actualVDS != 0 {
				t.Fatalf("sign-ins %d, Metadata requests %d, VDS requests %d", actualSignIns, actualGraph, actualVDS)
			}
			audit, err := os.ReadFile(filepath.Join(h.ws, "audit.md"))
			if err != nil {
				t.Fatal(err)
			}
			draft, err := os.ReadFile(filepath.Join(h.ws, "draft.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "complete", "partial", "schema-only":
				for _, name := range []string{"[Revenue] - [Cost]", "Index only", "upstream-only", "Physical only", "Protected: [REDACTED]", "Session: [REDACTED]", "PAT: *********"} {
					if !strings.Contains(string(audit), name) {
						t.Fatalf("missing redacted Metadata API observation %q", name)
					}
				}
				if !strings.Contains(string(audit), fmt.Sprintf(`"provider_complete": %t`, mode != "partial")) {
					t.Fatal("incorrect Metadata API coverage")
				}
				if strings.Contains(string(audit), "Unit cost") != (mode != "schema-only") || strings.Contains(string(audit), "Profit overview") != (mode != "schema-only") {
					t.Fatal("exported field dependencies and sheets do not match the authorized lineage scope")
				}
				if mode == "schema-only" && !strings.Contains(string(audit), "lineage.pull") {
					t.Fatal("audit omitted the lineage coverage limitation")
				}
			case "denied":
				if !strings.Contains(string(audit), "denied by managed policy") {
					t.Fatal("policy omission hidden")
				}
			case "unavailable":
				if !strings.Contains(string(audit), "Metadata API enrichment was unavailable.") {
					t.Fatal("missing index access was hidden")
				}
			}
			for _, secret := range []string{"secret-token", "test-name", "test-secret"} {
				if strings.Contains(string(audit), secret) {
					t.Fatal("credential in audit")
				}
			}
			if strings.Contains(string(draft), "Index only") {
				t.Fatal("metadata-only name escaped into edit target template")
			}
			// The hermetic checkout can populate inventory infrastructure. It must never
			// populate cached datasource schema as a side effect of an audit.
			verifyAuditSchemaCacheEmpty(t, filepath.Dir(h.ws))
		})
	}
}

func verifyAuditSchemaCacheEmpty(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Name() != "catalog.sqlite" {
			return err
		}
		db, err := sql.Open("sqlite", path+"?mode=ro")
		if err != nil {
			return err
		}
		defer db.Close()
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM resource_entries WHERE kind IN ('datasource_schema','datasource_schema_metadata')").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			t.Errorf("audit cached %d published schema observations", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
