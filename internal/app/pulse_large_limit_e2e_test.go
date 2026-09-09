package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestPulseListsLargerLimitUsesBoundedProviderPagesThroughCLI(t *testing.T) {
	for _, kind := range []string{"definition", "metric"} {
		t.Run(kind, func(t *testing.T) {
			pages := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/3.29/auth/signin" {
					_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					return
				}
				pages++
				size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
				if size < 1 || size > 100 {
					t.Errorf("provider page size=%d", size)
				}
				offset, _ := strconv.Atoi(r.URL.Query().Get("page_token"))
				items := []any{}
				for i := offset; i < offset+size; i++ {
					id := fmt.Sprintf("item-%03d", i)
					if kind == "definition" {
						items = append(items, map[string]any{"metadata": map[string]any{"id": id, "name": id}, "specification": map[string]any{"datasource": map[string]any{"id": "ds"}}})
					} else {
						items = append(items, map[string]any{"id": id, "definition_id": "definition-1"})
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{kind + "s": items, "next_page_token": strconv.Itoa(offset + size)})
			}))
			defer server.Close()
			args := []string{"pulse", kind, "list", "--limit", "150"}
			if kind == "metric" {
				args = append(args, "--definition-id", "definition-1")
			}
			var output bytes.Buffer
			code := app.Run(context.Background(), args, &output, pulseEfficiencyOptions(t, server))
			if code != 0 || pages != 2 || !strings.Contains(output.String(), "returned: 150") || !strings.Contains(output.String(), "more_available: true") {
				t.Fatalf("code=%d pages=%d output=%s", code, pages, output.String())
			}
		})
	}
}
