package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	session "github.com/ahillspace/tadx/actions/session"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/value"
)

func overviewService(cfg config.Config, loads *int) *session.Service {
	return session.New(session.Dependencies{
		ReadConfiguration: func() (config.Config, error) { *loads++; return cfg, nil },
		SiteSetting: func(config.Config, config.Environment) (value.MutationSetting, error) {
			return value.MutationSetting{}, nil
		},
		LookupEnv: coreauth.LookupEnvFunc(func(string) (string, bool) { return "", false }),
		ResolveWorkspace: func(context.Context, config.Config, string, string) (session.WorkspaceResolution, error) {
			return session.WorkspaceResolution{}, os.ErrNotExist
		},
	})
}

func overviewConfig(count int) config.Config {
	cfg := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{}, Workspaces: map[string]config.WorkspaceRegistration{}}
	for i := count - 1; i >= 0; i-- {
		name := fmt.Sprintf("env-%03d", i)
		cfg.Environments[name] = config.Environment{URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}}
		workspace := fmt.Sprintf("workspace-%03d", i)
		cfg.Workspaces[workspace] = config.WorkspaceRegistration{Path: "workspaces/example"}
	}
	return cfg
}

func TestOverviewBoundsBothProjectionsAndPreservesTruthfulTotals(t *testing.T) {
	loads := 0
	o, err := overviewService(overviewConfig(111), &loads).Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	compact := o.CompactOutput().(session.Result[session.CompactEnvironment, session.CompactWorkspace])
	full := o.FullOutput().(session.Result[session.Environment, session.Workspace])
	if compact.Environments.Returned != 10 || compact.Environments.Total != 111 || !compact.Environments.More || compact.Workspaces.Returned != 10 || !compact.Workspaces.More {
		t.Fatalf("compact = %#v", compact)
	}
	if full.Environments.Returned != 100 || full.Environments.Total != 111 || !full.Environments.More || full.Workspaces.Returned != 100 || !full.Workspaces.More {
		t.Fatalf("full = %#v", full)
	}
	if compact.Environments.Items[0].Name != "env-000" || full.Workspaces.Items[0].Name != "workspace-000" {
		t.Fatal("overview is not deterministically sorted")
	}
	if compact.AuthVerification != "not_checked" || full.AuthVerification != "not_checked" || loads != 1 {
		t.Fatal("projection repeated setup or claimed authentication")
	}
}

func TestOverviewEmptyAndExactProjectionBounds(t *testing.T) {
	for _, count := range []int{0, 10, 100} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			loads := 0
			o, err := overviewService(overviewConfig(count), &loads).Execute(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			compact := o.CompactOutput().(session.Result[session.CompactEnvironment, session.CompactWorkspace])
			full := o.FullOutput().(session.Result[session.Environment, session.Workspace])
			if compact.Environments.Returned != min(count, 10) || compact.Workspaces.Returned != min(count, 10) || compact.Environments.More != (count > 10) || compact.Workspaces.More != (count > 10) {
				t.Fatalf("compact bounds for %d: %#v", count, compact)
			}
			if full.Environments.Returned != count || full.Workspaces.Returned != count || full.Environments.More || full.Workspaces.More {
				t.Fatalf("full bounds for %d: %#v", count, full)
			}
			if count == 0 {
				for _, result := range []any{compact, full} {
					encoded, err := json.Marshal(result)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(encoded, []byte(`"items":[]`)) || bytes.Contains(encoded, []byte(`"items":null`)) {
						t.Fatalf("empty overview items changed shape: %s", encoded)
					}
				}
			}
		})
	}
}

func TestOverviewCancellationDoesNotReadSetup(t *testing.T) {
	loads := 0
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := overviewService(overviewConfig(0), &loads).Execute(ctx); err != context.Canceled {
		t.Fatalf("error = %v", err)
	}
	if loads != 0 {
		t.Fatal("read canceled setup")
	}
}
