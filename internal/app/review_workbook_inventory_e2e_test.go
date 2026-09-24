package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReviewProjectWorkbookAllScansOnceThroughCLI(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(fmt.Sprintf("drift=%t", drift), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/projects"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><projects><project id="project-1" name="Ops"/><project id="project-2" name="Other"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
				case strings.HasSuffix(r.URL.Path, "/workbooks"):
					calls.Add(1)
					if strings.Contains(r.URL.Query().Get("filter"), "projectId") {
						t.Error("unsupported projectId filter")
					}
					number, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
					size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
					if number < 1 || size < 1 {
						http.Error(w, "invalid page", 400)
						return
					}
					total := 5000
					if drift && number > 1 {
						total++
					}
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><workbooks>`, number, size, total)
					for index := (number - 1) * size; index < min(number*size, 5000); index++ {
						project := "project-2"
						if index%2 == 0 {
							project = "project-1"
						}
						fmt.Fprintf(w, `<workbook id="wb-%04d" name="Workbook %04d"><project id="%s"/><owner id="user-1"/></workbook>`, index, index, project)
					}
					fmt.Fprint(w, `</workbooks></tsResponse>`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			code, payload := resultContractJSON(t, []string{"content", "workbook", "list", "--environment", "test", "--project-id", "project-1", "--all"}, diagnosticOptions(t, server))
			t.Logf("workbook inventory GETs=%d", calls.Load())
			if drift {
				if code == 0 {
					t.Fatalf("pagination drift accepted: %#v", payload)
				}
				return
			}
			if code != 0 {
				t.Fatalf("list failed: %#v", payload)
			}
			items, ok := payload["workbooks"].([]any)
			if !ok || len(items) != 2500 {
				t.Fatalf("returned %d rows", len(items))
			}
			seen := map[string]bool{}
			for _, raw := range items {
				item := contractObject(t, raw)
				id, _ := item["luid"].(string)
				index, err := strconv.Atoi(strings.TrimPrefix(id, "wb-"))
				if err != nil || index < 0 || index >= 5000 || index%2 != 0 || seen[id] || item["project_luid"] != "project-1" {
					t.Fatalf("incorrect identity: %#v", item)
				}
				seen[id] = true
			}
			if calls.Load() != 5 {
				t.Fatalf("workbook inventory GETs=%d, want 5", calls.Load())
			}
		})
	}
}

func TestReviewCachedWorkbookNameIsListFilterThroughCLI(t *testing.T) {
	for _, population := range []string{"refresh", "read-through"} {
		t.Run(population, func(t *testing.T) {
			var blocked atomic.Bool
			books := []string{
				`<workbook id="wb-1" name="Finance"><project id="project-1"/><owner id="user-1"/></workbook>`,
				`<workbook id="wb-2" name="Finance"><project id="project-2"/><owner id="user-1"/></workbook>`,
				`<workbook id="wb-3" name="Finance"><project id="project-1"/><owner id="user-1"/></workbook>`,
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if blocked.Load() {
					t.Errorf("cache contacted Tableau: %s", r.URL)
					http.Error(w, "blocked", 500)
					return
				}
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/projects"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><projects><project id="project-1" name="Ops"/><project id="project-2" name="Other"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
				case strings.HasSuffix(r.URL.Path, "/workbooks"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="3"/><workbooks>%s</workbooks></tsResponse>`, r.URL.Query().Get("pageSize"), strings.Join(books, ""))
				case strings.Contains(r.URL.Path, "/workbooks/wb-"):
					index, err := strconv.Atoi(r.URL.Path[len(r.URL.Path)-1:])
					if err != nil || index < 1 || index > len(books) {
						http.NotFound(w, r)
						return
					}
					fmt.Fprintf(w, `<tsResponse>%s</tsResponse>`, books[index-1])
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			if population == "refresh" {
				code, payload := resultContractJSON(t, []string{"cache", "refresh", "--environment", "test", "--scope", "workbooks"}, options)
				if code != 0 {
					t.Fatalf("refresh: %#v", payload)
				}
			} else {
				for index := 1; index <= 3; index++ {
					code, payload := resultContractJSON(t, []string{"content", "workbook", "inspect", "--environment", "test", "--id", fmt.Sprintf("wb-%d", index)}, options)
					if code != 0 {
						t.Fatalf("read-through: %#v", payload)
					}
				}
			}
			blocked.Store(true)
			listArgs := []string{"content", "workbook", "list", "--environment", "test", "--cache", "--name", "Finance", "--limit", "10"}
			code, payload := resultContractJSON(t, listArgs, options)
			if code != 0 {
				t.Fatalf("name-filtered list: %#v", payload)
			}
			if items, ok := payload["workbooks"].([]any); !ok || len(items) != 3 {
				t.Fatalf("name-filtered rows: %#v", payload)
			}
			code, payload = resultContractJSON(t, []string{"content", "workbook", "inspect", "--environment", "test", "--cache", "--name", "Finance", "--project-id", "project-1"}, options)
			if code == 0 || !strings.Contains(fmt.Sprint(payload), "ambiguous") {
				t.Fatalf("inspection must remain ambiguous: %#v", payload)
			}
			if population == "refresh" {
				code, payload = resultContractJSON(t, []string{"content", "workbook", "list", "--environment", "test", "--cache", "--name", "Missing"}, options)
				if code != 0 {
					t.Fatalf("empty complete filter: %#v", payload)
				}
				if items, ok := payload["workbooks"].([]any); !ok || len(items) != 0 {
					t.Fatalf("expected empty rows: %#v", payload)
				}
				code, payload = resultContractJSON(t, []string{"content", "workbook", "list", "--environment", "test", "--cache", "--name", "Finance", "--limit", "1"}, options)
				if code != 0 || contractObject(t, payload["page"])["more_available"] != true {
					t.Fatalf("bounded name-filter page: %#v", payload)
				}
				code, payload = resultContractJSON(t, []string{"content", "workbook", "list", "--environment", "test", "--cache", "--name", "Finance", "--all"}, options)
				if code != 0 {
					t.Fatalf("all name-filtered rows: %#v", payload)
				}
				if items, ok := payload["workbooks"].([]any); !ok || len(items) != 3 {
					t.Fatalf("all name-filtered rows: %#v", payload)
				}
			}
		})
	}
}
