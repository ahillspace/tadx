package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestCapabilityListPreservesStaticFullRowsWhenPolicyConfigurationFails(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configuration := `version: 1
default_environment: selected
environments:
  selected:
    url: https://tableau.example.com
    site_content_url: selected
    auth:
      type: pat
      pat_name_env: SELECTED_PAT_NAME
      pat_secret_env: SELECTED_PAT_SECRET
  unused-invalid:
    url: https://tableau.example.com
    auth:
      type: basic
`
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}

	var first bytes.Buffer
	for _, extra := range [][]string{nil, {"--mutation=false"}} {
		args := []string{"capability", "list", "--limit", "1", "--full", "--json"}
		args = append(args, extra...)
		first.Reset()
		code := app.Run(context.Background(), args, &first, app.Options{ConfigPath: configPath})
		if code != 1 {
			t.Fatalf("list args=%v exit=%d output=%s", args, code, first.String())
		}
		if !strings.Contains(first.String(), `"mutation_policy":"unavailable"`) {
			t.Fatalf("list args=%v omitted unavailable policy: %s", args, first.String())
		}
	}
	var envelope struct {
		Output struct {
			Capabilities []struct {
				ID        string   `json:"id"`
				Selectors []string `json:"selectors"`
			} `json:"capabilities"`
			MutationPolicy string `json:"mutation_policy"`
		} `json:"output"`
		Error struct {
			Summary       string `json:"summary"`
			UpstreamCause string `json:"upstream_cause"`
		} `json:"error"`
	}
	if err := json.Unmarshal(first.Bytes(), &envelope); err != nil {
		t.Fatalf("list output is not JSON: %v\n%s", err, first.String())
	}
	if len(envelope.Output.Capabilities) != 1 || envelope.Output.Capabilities[0].ID == "" || len(envelope.Output.Capabilities[0].Selectors) == 0 {
		t.Fatalf("full static output = %#v", envelope.Output)
	}
	cause := envelope.Error.Summary + " " + envelope.Error.UpstreamCause
	expectedCause := `environment "unused-invalid" auth type must be "pat"`
	if envelope.Output.MutationPolicy != "unavailable" || !strings.Contains(cause, expectedCause) {
		t.Fatalf("policy diagnostic = %#v error=%#v", envelope.Output, envelope.Error)
	}

	var saved bytes.Buffer
	code := app.Run(context.Background(), []string{"last", "--full", "--json"}, &saved, app.Options{ConfigPath: configPath})
	if code != 0 {
		t.Fatalf("last exit=%d output=%s", code, saved.String())
	}
	if !strings.Contains(saved.String(), `"selectors"`) || !strings.Contains(saved.String(), `"mutation_policy":"unavailable"`) || !strings.Contains(saved.String(), `"error"`) {
		t.Fatalf("saved partial output lost detail or error: %s", saved.String())
	}
}
