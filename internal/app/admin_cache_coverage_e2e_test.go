package app_test

import (
	"context"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminAllRejectsPartialCacheThroughCLI(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("cache contacted remote %s", r.URL)
				w.WriteHeader(500)
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			store := targetCacheFixture(t, options.ConfigPath, nil)
			err := store.UpsertResources(context.Background(), []cache.ResourceEntry{{Environment: "production", Kind: kind, LUID: "known", Name: "Known", Coverage: "summary", ObservedAt: time.Now(), Payload: []byte(`{"luid":"known","name":"Known"}`)}})
			if err != nil {
				t.Fatal(err)
			}
			for _, all := range []bool{false, true} {
				var out strings.Builder
				args := []string{"admin", kind, "list", "--cache", "--environment", "production"}
				if all {
					args = append(args, "--all")
				}
				exit := app.Run(context.Background(), args, &out, options)
				if all && exit == 0 || !all && exit != 0 {
					t.Fatalf("all=%t exit=%d output=%s", all, exit, out.String())
				}
			}
		})
	}
}
