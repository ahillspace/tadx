package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestAdminRequestedThousandUsesTenBoundedPagesWithoutCache(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			reads, signins := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					signins++
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/"+kind+"s") {
					http.Error(w, "unexpected request", 500)
					return
				}
				reads++
				number, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
				size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
				if size != 100 || number != reads {
					t.Errorf("unbounded or unordered request: %s", r.URL.RawQuery)
				}
				fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="1500"/><%ss>`, number, size, kind)
				for index := (number - 1) * size; index < number*size; index++ {
					fmt.Fprintf(w, `<%s id="item-%d" name="Item %04d" siteRole="Viewer"/>`, kind, index, index)
				}
				fmt.Fprintf(w, `</%ss></tsResponse>`, kind)
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			var out strings.Builder
			exit := app.Run(context.Background(), []string{"admin", kind, "list", "--environment", "test", "--limit", "1000"}, &out, options)
			if exit != 0 || reads != 10 || signins != 1 || !strings.Contains(out.String(), "returned: 1000") || !strings.Contains(out.String(), "more_available: true") || strings.Contains(out.String(), "item-1000,") {
				t.Fatalf("exit=%d reads=%d signin=%d output=%s", exit, reads, signins, out.String())
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(options.ConfigPath), "catalog")); !os.IsNotExist(err) {
				t.Fatalf("limited list touched cache: %v", err)
			}
		})
	}
}
