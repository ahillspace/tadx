package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueryLabelsUsesDocumentedReadOnlyPOSTAndCanonicalType(t *testing.T) {
	authorizations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/3.29/sites/site/labels" || r.Header.Get("Content-Type") != "application/xml" {
			t.Fatalf("unexpected label query request: %s %s %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		if !strings.Contains(string(body[:n]), `contentType="datasource" id="ds-1"`) {
			t.Fatalf("label query omitted exact content target: %s", body[:n])
		}
		_, _ = fmt.Fprint(w, `{"labelList":{"label":[{"id":"label-1","contentId":"ds-1","contentType":"DATASOURCE","message":"owned","value":"Certified","category":"certification","active":false,"elevated":false}]}}`)
	}))
	defer server.Close()
	api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(), AuthorizeWrite: func(_ context.Context, _, _ string) error { authorizations++; return nil }}
	labels, err := api.QueryLabels(t.Context(), "datasource", "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].ID != "label-1" || labels[0].Name != "owned" || labels[0].ProjectID != "ds-1" || authorizations != 0 {
		t.Fatalf("query result=%#v authorizations=%d", labels, authorizations)
	}
}

func TestQueryLabelsAcceptsOnlyExplicitEmptyContainers(t *testing.T) {
	for name, body := range map[string]string{
		"empty object": `{"labelList":{}}`,
		"empty array":  `{"labelList":[]}`,
		"empty rows":   `{"labelList":{"label":[]}}`,
		"empty XML":    `<tsResponse><labelList/></tsResponse>`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, body)
			}))
			defer server.Close()
			api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
			labels, err := api.QueryLabels(t.Context(), "datasource", "ds-1")
			if err != nil || len(labels) != 0 {
				t.Fatalf("explicit empty response labels=%v err=%v", labels, err)
			}
		})
	}
	for name, body := range map[string]string{
		"missing envelope":  `{"pagination":{}}`,
		"null envelope":     `{"labelList":null}`,
		"malformed rows":    `{"labelList":{"label":null}}`,
		"unrelated object":  `{"labelList":{"other":[]}}`,
		"nonempty XML row":  `<tsResponse><labelList><label id="x"/></labelList></tsResponse>`,
		"extra XML sibling": `<tsResponse><labelList/><other/></tsResponse>`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = fmt.Fprint(w, body)
			}))
			defer server.Close()
			api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
			if labels, err := api.QueryLabels(t.Context(), "datasource", "ds-1"); ErrorStatus(err) != InconclusiveStatus || labels != nil {
				t.Fatalf("malformed response became complete empty result: labels=%v err=%v", labels, err)
			}
		})
	}
}

func TestJournalResumesLabelCleanupWithExactScopedQuery(t *testing.T) {
	requests, deletes, authorizations := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site/labels":
			requests++
			body := make([]byte, 1024)
			n, _ := r.Body.Read(body)
			if !strings.Contains(string(body[:n]), `contentType="datasource" id="ds-1"`) {
				t.Fatalf("cleanup query was not scoped to parent: %s", body[:n])
			}
			if requests == 1 {
				_, _ = fmt.Fprint(w, `{"labelList":{"label":[{"id":"label-1","contentId":"ds-1","contentType":"DATASOURCE","message":"tadx-live-run-attachment","value":"Certified","category":"certification","active":false,"elevated":false}]}}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"labelList":{"label":[]}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site/labels/label-1":
			deletes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected cleanup request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(), AuthorizeWrite: func(_ context.Context, _, _ string) error { authorizations++; return nil }}
	journal, err := NewJournal(filepath.Join(t.TempDir(), "journal.json"), "run", "target", "tadx-live-run")
	if err != nil {
		t.Fatal(err)
	}
	if err := journal.AddIntent("label", "tadx-live-run-attachment", "ds-1"); err != nil {
		t.Fatal(err)
	}
	if err := journal.SetLabelExpectation("tadx-live-run-attachment", "ds-1", map[string]any{"contentType": "datasource", "contentId": "ds-1", "value": "Certified", "category": "certification", "message": "tadx-live-run-attachment", "active": false, "elevated": false}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJournal(journal.path, "target")
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Cleanup(t.Context(), api); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || deletes != 1 || authorizations != 1 || !loaded.Entries[0].Done || loaded.Entries[0].Object == nil || loaded.Entries[0].Object.ID != "label-1" {
		t.Fatalf("recovery requests=%d deletes=%d authorizations=%d entry=%#v", requests, deletes, authorizations, loaded.Entries[0])
	}
}

func TestConfirmedLabelCleanupAcceptsMutableFieldUpdates(t *testing.T) {
	const updatedRow = `{"id":"label-1","contentId":"ds-1","contentType":"DATASOURCE","message":"updated message","value":"Certified","category":"certification","active":true,"elevated":false}`
	requests, deletes, authorizations := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site/labels":
			requests++
			if requests == 1 {
				_, _ = fmt.Fprintf(w, `{"labelList":{"label":[%s]}}`, updatedRow)
				return
			}
			_, _ = fmt.Fprint(w, `{"labelList":{"label":[]}}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site/labels/label-1":
			deletes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected cleanup request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(), AuthorizeWrite: func(_ context.Context, _, _ string) error { authorizations++; return nil }}
	journal := testJournal(t)
	name := "tadx-live-safety-attachment"
	if err := journal.AddIntent("label", name, "ds-1"); err != nil {
		t.Fatal(err)
	}
	if err := (&Case{Journal: journal}).ConfirmReceipt("label", name, "ds-1", Object{
		ID: "label-1", Name: name, Kind: "label", ProjectID: "ds-1",
		Fields: map[string]any{"contentType": "datasource", "contentId": "ds-1", "value": "Certified", "category": "certification", "message": name, "active": false, "elevated": false},
	}); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadJournal(journal.path, journal.Target)
	if err != nil {
		t.Fatal(err)
	}
	if err := loaded.Cleanup(t.Context(), api); err != nil {
		t.Fatalf("confirmed LUID cleanup rejected updated mutable fields: %v", err)
	}
	if requests != 2 || deletes != 1 || authorizations != 1 || !loaded.Entries[0].Done {
		t.Fatalf("cleanup requests=%d deletes=%d authorizations=%d done=%t", requests, deletes, authorizations, loaded.Entries[0].Done)
	}
}

func TestConfirmedLabelCleanupRejectsWrongScopeAndDuplicateLUID(t *testing.T) {
	const row = `{"id":"label-1","contentId":"ds-1","contentType":"DATASOURCE","message":"updated message","value":"Certified","category":"certification","active":true,"elevated":false}`
	for name, rows := range map[string]string{
		"wrong parent": `[{"id":"label-1","contentId":"ds-other","contentType":"DATASOURCE","message":"updated message","value":"Certified","category":"certification","active":true,"elevated":false}]`,
		"wrong type":   `[{"id":"label-1","contentId":"ds-1","contentType":"TABLE","message":"updated message","value":"Certified","category":"certification","active":true,"elevated":false}]`,
		"duplicate ID": "[" + row + "," + row + "]",
	} {
		t.Run(name, func(t *testing.T) {
			queries, deletes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deletes++
					w.WriteHeader(http.StatusNoContent)
					return
				}
				queries++
				_, _ = fmt.Fprintf(w, `{"labelList":{"label":%s}}`, rows)
			}))
			defer server.Close()
			api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
			journal := testJournal(t)
			labelName := "tadx-live-safety-attachment"
			if err := journal.AddIntent("label", labelName, "ds-1"); err != nil {
				t.Fatal(err)
			}
			if err := (&Case{Journal: journal}).ConfirmReceipt("label", labelName, "ds-1", Object{
				ID: "label-1", Name: labelName, Kind: "label", ProjectID: "ds-1",
				Fields: map[string]any{"contentType": "datasource", "contentId": "ds-1", "value": "Certified", "category": "certification", "message": labelName, "active": false, "elevated": false},
			}); err != nil {
				t.Fatal(err)
			}
			if err := journal.Cleanup(t.Context(), api); ErrorStatus(err) != InconclusiveStatus {
				t.Fatalf("unsafe confirmed label response did not fail closed: %v", err)
			}
			if queries != 1 || deletes != 0 || journal.Entries[0].Done {
				t.Fatalf("unsafe response queries=%d deletes=%d done=%t", queries, deletes, journal.Entries[0].Done)
			}
		})
	}
}

func TestCreateLabelUsesExactScopedQueryWhenPutReceiptIsSparse(t *testing.T) {
	writes, reads, authorized := 0, 0, 0
	intentRecorded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site/labels":
			writes++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site/labels":
			reads++
			_, _ = fmt.Fprint(w, `{"labelList":{"label":[{"id":"label-2","contentId":"ds-2","contentType":"DATASOURCE","message":"tadx-live-run-att","value":"Certified","category":"certification","active":false,"elevated":false}]}}`)
		default:
			t.Fatalf("unexpected label create request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	api := &Client{
		Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(),
		AuthorizeWrite: func(_ context.Context, method, _ string) error {
			if method == http.MethodPut {
				authorized++
			}
			return nil
		},
		JournalLabelIntent: func(name, parent string, expected map[string]any) error {
			intentRecorded = name == "tadx-live-run-att" && parent == "ds-2" && expected["value"] == "Certified" && expected["category"] == "certification"
			return nil
		},
	}
	label, err := api.CreateLabel(t.Context(), LabelAttachment{ContentType: "datasource", ContentID: "ds-2", Value: "Certified", Category: "certification", Message: "tadx-live-run-att"})
	if err != nil {
		t.Fatal(err)
	}
	if label.ID != "label-2" || label.Name != "tadx-live-run-att" || writes != 1 || reads != 1 || authorized != 1 || !intentRecorded {
		t.Fatalf("label=%#v writes=%d reads=%d authorized=%d intent=%t", label, writes, reads, authorized, intentRecorded)
	}
}

func TestPendingLabelRecoveryRequiresOneExactTuple(t *testing.T) {
	row := `{"id":"label-3","contentId":"ds-3","contentType":"DATASOURCE","message":"tadx-live-run-att","value":"Certified","category":"certification","active":false,"elevated":false}`
	for name, rows := range map[string]string{
		"mismatch":  `[{"id":"label-3","contentId":"ds-3","contentType":"DATASOURCE","message":"other","value":"Certified","category":"certification","active":false,"elevated":false}]`,
		"duplicate": "[" + row + "," + row + "]",
		"empty":     "[]",
	} {
		t.Run(name, func(t *testing.T) {
			deletes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deletes++
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/api/3.29/sites/site/labels" {
					t.Fatalf("unexpected recovery request: %s %s", r.Method, r.URL.Path)
				}
				_, _ = fmt.Fprintf(w, `{"labelList":{"label":%s}}`, rows)
			}))
			defer server.Close()
			api := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
			journal, err := NewJournal(filepath.Join(t.TempDir(), "journal.json"), "run", "target", "tadx-live-run")
			if err != nil {
				t.Fatal(err)
			}
			if err = journal.AddIntent("label", "tadx-live-run-att", "ds-3"); err != nil {
				t.Fatal(err)
			}
			if err = journal.SetLabelExpectation("tadx-live-run-att", "ds-3", map[string]any{"contentType": "datasource", "contentId": "ds-3", "value": "Certified", "category": "certification", "message": "tadx-live-run-att", "active": false, "elevated": false}); err != nil {
				t.Fatal(err)
			}
			if err = journal.Cleanup(t.Context(), api); ErrorStatus(err) != InconclusiveStatus || deletes != 0 {
				t.Fatalf("unsafe recovery result=%v status=%q deletes=%d", err, ErrorStatus(err), deletes)
			}
		})
	}
}

func TestLabelResponseRowsRejectMalformedEnvelope(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal([]byte(`{"labelList":{"label":[null]}}`), &document); err != nil {
		t.Fatal(err)
	}
	rows, ok := labelResponseRows(document)
	if !ok || len(rows) != 0 {
		t.Fatalf("malformed rows parsed as %#v, present=%t", rows, ok)
	}
}
