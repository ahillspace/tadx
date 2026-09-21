package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestPublicationWorkerRechecksCurrentSiteConsent(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected remote request", 500)
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	options := withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), Stderr: io.Discard}, true)
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish", ConfigPath: options.ConfigPath, Args: []string{"content", "workbook", "publish", "--environment", "production", "--file", filepath.Join(t.TempDir(), "unused.twb"), "--project-id", "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Update(options.ConfigPath, false, func(c config.Config) (config.Config, error) {
		environment, err := c.ResolveEnvironment("production")
		if err != nil {
			return c, err
		}
		err = c.SetMutationSetting(environment, false)
		return c, err
	}); err != nil {
		t.Fatal(err)
	}
	if code := runPublicationWorker(t.Context(), store.Directory, record.ID, options); code == 0 {
		t.Fatal("worker ignored revoked site consent")
	}
	result, err := store.Read(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 || !strings.Contains(string(result.FullResult), "mutation.disabled") {
		t.Fatalf("requests=%d result=%s", requests.Load(), result.FullResult)
	}
}
