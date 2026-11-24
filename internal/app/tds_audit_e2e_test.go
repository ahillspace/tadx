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
				// The indexed field remains useful when the provider flags partial results.
				if mode == "unavailable" {
					w.WriteHeader(403)
					io.WriteString(w, `{"error":{"code":"403004","summary":"Unavailable"}}`)
					return true
				}
				issues := ""
				if mode == "partial" {
					issues = `"errors":[{"message":"Provider returned partial observations"}],`
				}
				fmt.Fprintf(w, `{%s"data":{"parents":{"nodes":[{
 "id":"source-graph","luid":%q,"name":"Indexed source","description":"Protected: test-secret",
 "tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},
 "items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{
   "id":"index-only","name":"Index only","fullyQualifiedName":"[Only in the index]",
   %s"description":"Session: secret-token","descriptionInherited":[],
   "upstreamColumnsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{
     "id":"upstream-only","luid":"column-id","name":"Physical only","description":"PAT: test-name"
   }]}
 }]}
}]}}}`, issues, fixtureTDSOriginal, extras)
				return true
			}
			h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "audit.md", "--changes-file", "draft.yaml")
			h.fixture.mu.Lock()
			actualSignIns, actualGraph, actualVDS := signIns, graph, vdscalls
			h.fixture.mu.Unlock()
			expected := map[string]int{"complete": 1, "schema-only": 1, "denied": 0, "unavailable": 1, "partial": 1}[mode]
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

func TestTDSExplicitTemplateFieldsReuseCollectionEvidence(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	var selectors []string
	for i := range 50 {
		name := fmt.Sprintf("Field %02d", i)
		h.fixture.source[tdsEntryKey("Column", name)] = value.TDSElement{Name: name, Column: &value.TDSColumn{Caption: new(name)}}
		h.fixture.source[tdsEntryKey("FieldDescription", name)] = value.TDSElement{Name: name, Description: new("Observed")}
		selectors = append(selectors, "--template-field", name)
	}
	var reads []string
	h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/workcopy/") {
			q := r.URL.Query()
			reads = append(reads, q.Get("elementType")+"/"+q.Get("name"))
		}
		return false
	}
	h.ds(true, append([]string{"checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "audit.md", "--changes-file", "template.yaml"}, selectors...)...)
	h.fixture.mu.Lock()
	got := strings.Join(reads, ",")
	h.fixture.mu.Unlock()
	if got != "Column/,FieldDescription/,TableDescription/" {
		t.Errorf("expected just the three collection reads, got %s", got)
	}
	template, err := os.ReadFile(filepath.Join(h.ws, "template.yaml"))
	if err != nil || strings.Count(string(template), "#     - op: set-field-description") != 50 {
		t.Fatalf("generated examples: %v, %s", err, template)
	}
}

func TestTDSTemplateNamedFallbackForOmittedField(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	const selected = "Outside projection"
	h.fixture.source[tdsEntryKey("Column", selected)] = value.TDSElement{Name: selected, Column: &value.TDSColumn{Caption: new(selected)}}
	var reads []string
	h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != "GET" || !strings.Contains(r.URL.Path, "/workcopy/") {
			return false
		}
		q := r.URL.Query()
		reads = append(reads, q.Get("elementType")+"/"+q.Get("name"))
		if q.Get("elementType") != "Column" || q.Get("name") != "" {
			return false
		}
		var rows []value.TDSElement
		for i := range 100 {
			name := fmt.Sprintf("Other %03d", i)
			rows = append(rows, value.TDSElement{Name: name, Column: &value.TDSColumn{Caption: new(name)}})
		}
		rows = append(rows, h.fixture.source[tdsEntryKey("Column", selected)])
		if err := json.NewEncoder(w).Encode(map[string]any{"tdsElementList": rows}); err != nil {
			t.Error(err)
		}
		return true
	}
	h.ds(true, "checkout", "--id", fixtureTDSOriginal)
	h.ds(true, "work-copy", "--work-copy", fixtureTDSCopy, "--workspace", "work", "--audit-file", "audit.md", "--changes-file", "template.yaml", "--template-field", selected)
	h.fixture.mu.Lock()
	got := strings.Join(reads, ",")
	h.fixture.mu.Unlock()
	if got != "Column/,FieldDescription/,TableDescription/,Column/"+selected+",FieldDescription/"+selected {
		t.Errorf("unexpected named reads: %s", got)
	}
	for _, path := range []string{"audit.md", "template.yaml"} {
		file, err := os.ReadFile(filepath.Join(h.ws, path))
		if err != nil || !strings.Contains(string(file), selected) {
			t.Fatalf("missing verified selector in %s: %v", path, err)
		}
	}
}

func TestTDSAuditLimitsMetadataAcquisitionAndReportsCoverage(t *testing.T) {
	for _, mode := range []string{"lineage", "schema-only", "partial"} {
		t.Run(mode, func(t *testing.T) {
			h := newTDSWorkflowCLI(t)
			h.options = withSiteMutationConsent(t, h.options, true)
			if mode == "schema-only" {
				delete(h.options.managedPolicy.(fixtureManagedPolicy).allowed, "lineage.pull")
			}
			requests := 0
			h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
				if !strings.Contains(r.URL.Path, "metadata") {
					return false
				}
				requests++
				var body struct {
					Query     string
					Variables map[string]any
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return true
				}
				if requests != 1 || body.Variables["first"] != float64(100) || body.Variables["parent"] != fixtureTDSOriginal || body.Variables["after"] != nil {
					w.WriteHeader(http.StatusForbidden)
					return true
				}
				if strings.Contains(body.Query, "referencedFields:fieldsConnection") != (mode != "schema-only") || !strings.Contains(body.Query, "formula") {
					t.Errorf("unexpected audit query: %s", body.Query)
				}
				page := func(rows []any, total int) map[string]any {
					return map[string]any{"nodes": rows, "totalCount": total, "pageInfo": map[string]any{"hasNextPage": total > len(rows), "endCursor": "outside-audit"}}
				}
				var fields []any
				for i := range 100 {
					name := fmt.Sprintf("field-%02d", i)
					field := map[string]any{"id": name, "name": name, "fullyQualifiedName": "[" + name + "]", "__typename": "CalculatedField", "formula": "1+2", "descriptionInherited": []any{}}
					for _, rel := range []struct {
						key   string
						extra int
					}{{"upstreamColumnsConnection", 3}, {"referencedFields", 4}, {"relatedSheets", 5}} {
						if mode == "schema-only" && rel.key != "upstreamColumnsConnection" {
							continue
						}
						rows := []any{}
						total := 0
						if i == 0 {
							for j := range 100 {
								label := fmt.Sprintf("%s-%02d", rel.key, j)
								rows = append(rows, map[string]any{"id": label, "name": label})
							}
							total = len(rows) + rel.extra
						}
						field[rel.key] = page(rows, total)
					}
					if mode == "partial" && i == 1 {
						field["relatedSheets"] = nil
						field["descriptionInherited"] = []any{map[string]any{"value": "missing provenance"}}
						field["referencedFields"] = map[string]any{"nodes": []any{map[string]any{"id": "visible", "name": "Identified reference"}}, "pageInfo": map[string]any{"hasNextPage": false}}
					}
					fields = append(fields, field)
				}
				node := map[string]any{"id": "source", "luid": fixtureTDSOriginal, "name": "Source", "items": page(fields, 102)}
				if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"parents": map[string]any{"nodes": []any{node}}}}); err != nil {
					t.Error(err)
				}
				return true
			}
			h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "audit.md")
			h.fixture.mu.Lock()
			gotRequests := requests
			h.fixture.mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("Metadata requests = %d, want one bounded query", gotRequests)
			}
			audit, err := os.ReadFile(filepath.Join(h.ws, "audit.md"))
			if err != nil {
				t.Fatal(err)
			}
			_, section, found := strings.Cut(string(audit), "## Published datasource Metadata API context\n\n")
			if !found {
				t.Fatal("missing indexed datasource observation")
			}
			section, _, _ = strings.Cut(section, "\n\n")
			type coverage struct {
				Total   *int  `json:"total_count"`
				More    *bool `json:"has_next_page"`
				Omitted *int  `json:"omitted"`
			}
			var result struct {
				ProviderComplete bool     `json:"provider_complete"`
				FieldsCoverage   coverage `json:"fields_coverage"`
				Fields           []struct {
					value.FieldDescription
					Columns       coverage  `json:"upstream_columns_coverage"`
					References    *coverage `json:"referenced_fields_coverage"`
					SheetCoverage *coverage `json:"sheets_coverage"`
				} `json:"fields"`
			}
			if err = json.Unmarshal([]byte(section), &result); err != nil {
				t.Fatal(err)
			}
			check := func(name string, got *coverage, total, omitted int) {
				t.Helper()
				if got == nil || got.Total == nil || *got.Total != total || got.More == nil || !*got.More || got.Omitted == nil || *got.Omitted != omitted {
					t.Errorf("missing %s coverage (%d total, %d omitted): %+v", name, total, omitted, got)
				}
			}
			check("fields", &result.FieldsCoverage, 102, 2)
			if len(result.Fields) != 100 {
				t.Fatalf("retained fields %d", len(result.Fields))
			}
			field := result.Fields[0]
			if field.Formula != "1+2" || len(field.UpstreamColumns) != 100 {
				t.Fatalf("missing indexed formula or columns")
			}
			check("columns", &field.Columns, 103, 3)
			if mode == "schema-only" {
				if field.References != nil || field.SheetCoverage != nil || len(field.ReferencedFields) != 0 || len(field.Sheets) != 0 {
					t.Fatal("lineage scope included in schema-only response")
				}
			} else {
				check("references", field.References, 104, 4)
				check("sheets", field.SheetCoverage, 105, 5)
				if len(field.ReferencedFields) != 100 || field.ReferencedFields[99].Name != "referencedFields-99" || len(field.Sheets) != 100 || field.Sheets[99].Name != "relatedSheets-99" {
					t.Fatal("the projected references or sheets were lost")
				}
			}
			if result.ProviderComplete != (mode != "partial") {
				t.Fatalf("provider success conflated with projected truncation")
			}
			if mode == "partial" {
				missing := result.Fields[1].SheetCoverage
				if missing == nil || missing.Total != nil || missing.More != nil || missing.Omitted != nil || result.Fields[1].Formula != "1+2" || result.Fields[1].InheritedObserved || result.Fields[99].Name != "field-99" {
					t.Fatalf("partial response claimed unknown counts or dropped valid observations")
				}
				refs := result.Fields[1].References
				if refs == nil || refs.Total != nil || refs.Omitted != nil || refs.More == nil || *refs.More || len(result.Fields[1].ReferencedFields) != 1 || result.Fields[1].ReferencedFields[0].Name != "Identified reference" {
					t.Fatal("missing total dropped independent identities or known page information")
				}
				if !strings.Contains(section, `"omitted": null`) || !strings.Contains(string(audit), "partial published datasource observations") || !strings.Contains(string(audit), "continuation outside the audit") {
					t.Fatal("provider failure not reported")
				}
			}
			verifyAuditSchemaCacheEmpty(t, filepath.Dir(h.ws))
		})
	}
}

func TestTDSTemplateFallbackPreservesReadFailures(t *testing.T) {
	for _, mode := range []string{"ambiguous-column", "failed-description", "unconfirmed-absence"} {
		t.Run(mode, func(t *testing.T) {
			h := newTDSWorkflowCLI(t)
			h.options = withSiteMutationConsent(t, h.options, true)
			var named []string
			h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/workcopy/") {
					return false
				}
				q := r.URL.Query()
				kind := q.Get("elementType")
				if q.Get("name") != "" {
					named = append(named, kind+"/"+q.Get("name"))
					if mode == "unconfirmed-absence" && kind == "FieldDescription" {
						w.WriteHeader(http.StatusNotFound)
						io.WriteString(w, `{"error":{"code":"404000","summary":"Work copy unavailable"}}`)
						return true
					}
					return false
				}
				switch {
				case mode == "ambiguous-column" && kind == "Column":
					io.WriteString(w, `{"tdsElementList":[{"name":"Sales","column":{"caption":"First"}},{"name":"Sales","column":{"caption":"Second"}}]}`)
				case mode == "failed-description" && kind == "FieldDescription":
					io.WriteString(w, `{"tdsElementList":[{"name":"Sales","error":{"code":7,"message":"Denied"}}]}`)
				default:
					return false
				}
				return true
			}
			got := h.ds(false, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--changes-file", "draft.yaml", "--audit-file", "audit.md", "--template-field", "Sales")
			requireTDSJSON(t, got, fixtureTDSCopy, `"audit_file":"audit.md"`)
			h.fixture.mu.Lock()
			reads := strings.Join(named, ",")
			h.fixture.mu.Unlock()
			if reads != "Column/Sales,FieldDescription/Sales" {
				t.Errorf("unexpected fallback reads %s", reads)
			}
			draft, err := os.ReadFile(filepath.Join(h.ws, "draft.yaml"))
			if mode == "unconfirmed-absence" {
				if !os.IsNotExist(err) || !strings.Contains(got, "--changes-file") {
					t.Fatal("template accepted an unconfirmed absence or omitted its recovery command")
				}
			} else if err != nil || !strings.Contains(string(draft), "field: Sales") {
				t.Fatalf("confirmed named observations lost after collection failure: %v", err)
			}
		})
	}
}
