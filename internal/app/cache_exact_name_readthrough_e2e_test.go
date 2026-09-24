package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLiveWorkbookReadThroughKeepsExactNameIndexThroughCLI(t *testing.T) {
	var blockNetwork atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if blockNetwork.Load() {
			t.Errorf("cached inspection contacted Tableau: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected cache network request", http.StatusInternalServerError)
			return
		}
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/projects":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Ops"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
		case "/api/3.29/sites/site-1/workbooks":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><workbooks><workbook id="wb-plain" name="Finance"><project id="project-1" name="Ops"/></workbook><workbook id="wb-space" name="Finance "><project id="project-1" name="Ops"/></workbook></workbooks></tsResponse>`, r.URL.Query().Get("pageSize"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	for _, want := range []struct{ name, luid string }{{"Finance", "wb-plain"}, {"Finance ", "wb-space"}} {
		args := []string{"content", "workbook", "inspect", "--environment", "test", "--name", want.name, "--project-id", "project-1"}
		code, payload := resultContractJSON(t, args, options)
		if code != 0 {
			t.Fatalf("live inspect %q exit=%d payload=%#v", want.name, code, payload)
		}
		workbook := contractObject(t, payload["workbook"])
		if workbook["luid"] != want.luid || workbook["name"] != want.name {
			t.Fatalf("live inspect %q returned %#v", want.name, workbook)
		}
	}
	blockNetwork.Store(true)
	for _, want := range []struct{ name, luid string }{{"Finance", "wb-plain"}, {"Finance ", "wb-space"}} {
		args := []string{"content", "workbook", "inspect", "--environment", "test", "--name", want.name, "--project-id", "project-1", "--cache"}
		code, payload := resultContractJSON(t, args, options)
		if code != 0 {
			t.Fatalf("cached exact-name inspect %q exit=%d payload=%#v", want.name, code, payload)
		}
		workbook := contractObject(t, payload["workbook"])
		if workbook["luid"] != want.luid || workbook["name"] != want.name {
			t.Fatalf("cached exact-name inspect %q returned %#v", want.name, workbook)
		}
	}
}
