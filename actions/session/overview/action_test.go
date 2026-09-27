package overview_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/actions/session/overview"
)

type reader struct {
	state overview.State
	calls int
}

func (r *reader) ReadOverview(context.Context) (overview.State, error) {
	r.calls++
	return r.state, nil
}

func TestOverviewBoundsBothProjectionsAndPreservesTruthfulTotals(t *testing.T) {
	r := &reader{}
	for i := 110; i >= 0; i-- {
		r.state.Environments = append(r.state.Environments, overview.Environment{Name: fmt.Sprintf("env-%03d", i), ServerURL: "https://tableau.example.test", Credentials: "configured"})
		r.state.Workspaces = append(r.state.Workspaces, overview.Workspace{Name: fmt.Sprintf("workspace-%03d", i), Path: "workspaces/example"})
	}
	o, err := overview.New(r).Execute(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	compact := o.CompactOutput().(overview.Result[overview.CompactEnvironment, overview.CompactWorkspace])
	full := o.FullOutput().(overview.Result[overview.Environment, overview.Workspace])
	if compact.Environments.Returned != 10 || compact.Environments.Total != 111 || !compact.Environments.More || compact.Workspaces.Returned != 10 || !compact.Workspaces.More {
		t.Fatalf("compact = %#v", compact)
	}
	if full.Environments.Returned != 100 || full.Environments.Total != 111 || !full.Environments.More || full.Workspaces.Returned != 100 || !full.Workspaces.More {
		t.Fatalf("full = %#v", full)
	}
	if compact.Environments.Items[0].Name != "env-000" || full.Workspaces.Items[0].Name != "workspace-000" {
		t.Fatal("overview is not deterministically sorted")
	}
	if compact.AuthVerification != "not_checked" || full.AuthVerification != "not_checked" || r.calls != 1 {
		t.Fatal("projection repeated setup or claimed authentication")
	}
}

func TestOverviewEmptyAndExactProjectionBounds(t *testing.T) {
	for _, count := range []int{0, 10, 100} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			r := &reader{}
			for i := range count {
				r.state.Environments = append(r.state.Environments, overview.Environment{Name: fmt.Sprintf("env-%03d", i)})
				r.state.Workspaces = append(r.state.Workspaces, overview.Workspace{Name: fmt.Sprintf("workspace-%03d", i)})
			}
			o, err := overview.New(r).Execute(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			compact := o.CompactOutput().(overview.Result[overview.CompactEnvironment, overview.CompactWorkspace])
			full := o.FullOutput().(overview.Result[overview.Environment, overview.Workspace])
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
	r := &reader{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := overview.New(r).Execute(ctx); err != context.Canceled {
		t.Fatalf("error = %v", err)
	}
	if r.calls != 0 {
		t.Fatal("read canceled setup")
	}
}
