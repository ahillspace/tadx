package app

import (
	"context"
	"fmt"
	searchaction "github.com/ahillspace/tadx/actions/search"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestNativeSearchReportsTotalAndResultWindowWarning(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.URL.Path == "/api/-/search":
			page, err := strconv.Atoi(r.URL.Query().Get("page"))
			if err != nil {
				t.Error(err)
				http.Error(w, "invalid page", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprintf(w, `{"items":[{"content":{"luid":"wb-%d","contentType":"WORKBOOK","name":"Sales"}}],"limit":1,"pageIndex":%d,"startIndex":%d,"total":2001,"next":"/api/-/search?page=%d"}`, page, page, page, page+1)
		default:
			http.Error(w, "unexpected request", 404)
		}
	}))
	defer server.Close()
	out, err := newSearchCommands(inventoryListRuntime(t, server)).Execute(context.Background(), searchaction.Input{Environment: "production", Type: "workbook", Terms: "Sales", Limit: 1})
	if err != nil || out.Page.Total != 2001 || out.Page.NextCursor == "" || !strings.Contains(strings.Join(out.Warnings, " "), "2,000") {
		t.Fatalf("search result = %#v, %v", out, err)
	}
}
