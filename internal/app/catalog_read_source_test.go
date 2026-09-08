package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	workbookget "github.com/ahillspace/tadx/actions/workbook/inspect"
	"github.com/ahillspace/tadx/internal/catalog"
)

type failNetworkTransport struct{ calls int }

func (t *failNetworkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, errors.New("unexpected network request")
}

func TestWorkbookGetCatalogUsesNoAuthenticationOrNetwork(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "tadx.yaml")
	configuration := `version: 1
default_environment: production
environments:
  production:
    url: https://tableau.invalid
    site_content_url: marketing
    api_version: "3.29"
    auth:
      type: pat
      pat_name_env: MISSING_PAT_NAME
      pat_secret_env: MISSING_PAT_SECRET
`
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	item := workbookget.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Operations", Description: "catalog copy"}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	store := catalog.NewStore(root, func() time.Time { return now })
	if err := store.UpsertResources(context.Background(), []catalog.ResourceEntry{{Environment: "production", Site: "marketing", Kind: "workbook", LUID: item.LUID, Name: item.Name, ProjectPath: item.ProjectPath, Payload: payload, Coverage: "detail", ObservedAt: now}}); err != nil {
		t.Fatal(err)
	}
	transport := &failNetworkTransport{}
	runtime, err := newRuntime(Options{ConfigPath: configPath, HTTPClient: &http.Client{Transport: transport}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	input := workbookget.Input{Catalog: true}
	input.SetSelector("wb-1", "", "")
	output, err := newRemoteContentCommands(runtime).InspectWorkbook(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 0 {
		t.Fatalf("network calls = %d", transport.calls)
	}
	if output.Workbook.Description != "catalog copy" || output.Source == nil || output.Source.Mode != "catalog" || output.Source.Coverage != "complete" {
		t.Fatalf("output = %#v", output)
	}
}
