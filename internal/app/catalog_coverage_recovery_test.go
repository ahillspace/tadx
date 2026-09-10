package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workbookinspect "github.com/ahillspace/tadx/actions/workbook/inspect"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/config"
)

func TestCachedSummaryExplainsCoverageWithoutInventingPermissionDenial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, config.Config{Version: 1, Environments: map[string]config.Environment{"work": {URL: "https://example.invalid", SiteContentURL: "test", Auth: config.Auth{Type: "pat"}}}}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := targetCatalogFixture(t, path, func() time.Time { return now })
	payload, _ := json.Marshal(workbookinspect.Workbook{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1", ProjectPath: "Reports"})
	if err := store.UpsertResources(context.Background(), []catalog.ResourceEntry{{Environment: "work", Site: "test", Kind: "workbook", LUID: "wb-1", Name: "Sales", Coverage: "summary", ObservedAt: now, Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	network := &failNetworkTransport{}
	var out bytes.Buffer
	code := Run(context.Background(), []string{"content", "workbook", "inspect", "--id", "wb-1", "--env", "work", "--catalog", "--full"}, &out, Options{ConfigPath: path, HTTPClient: &http.Client{Transport: network}, Now: func() time.Time { return now }})
	if code != 0 || network.calls != 0 || !strings.Contains(out.String(), "coverage_reason: summary_only") || strings.Contains(out.String(), "access_denied") {
		t.Fatalf("code=%d calls=%d output=%s", code, network.calls, out.String())
	}
}
