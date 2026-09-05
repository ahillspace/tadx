package status_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	target authstatus.Target
	err    error
}

func (r resolver) Resolve(context.Context, string) (authstatus.Target, error) { return r.target, r.err }

type recordingResolver struct {
	target authstatus.Target
	alias  string
}

func (r *recordingResolver) Resolve(_ context.Context, alias string) (authstatus.Target, error) {
	r.alias = alias
	return r.target, nil
}

type lookup map[string]string

func (l lookup) LookupEnv(key string) (string, bool) { value, ok := l[key]; return value, ok }

func TestExecuteReportsReadyWithoutReturningPATValues(t *testing.T) {
	action := authstatus.New(resolver{target: fixture()}, lookup{"PROD_PAT_NAME": "token-name", "PROD_PAT_SECRET": "highly-secret"})
	got, err := action.Execute(context.Background(), authstatus.Input{Environment: "production"})
	if err != nil || got.Status != "ready" || !got.PATNamePresent || !got.PATSecretPresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.String(), "token-name") || strings.Contains(rendered.String(), "highly-secret") {
		t.Fatalf("secret leaked: %s", rendered.String())
	}
}

func TestExecuteReportsIncompleteForMissingOrEmptyValues(t *testing.T) {
	got, err := authstatus.New(resolver{target: fixture()}, lookup{"PROD_PAT_NAME": "  "}).Execute(context.Background(), authstatus.Input{})
	if err != nil || got.Status != "incomplete" || got.PATNamePresent || got.PATSecretPresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecutePreservesExactAlias(t *testing.T) {
	resolver := &recordingResolver{target: fixture()}
	_, err := authstatus.New(resolver, lookup{}).Execute(context.Background(), authstatus.Input{Environment: "Prod-West"})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.alias != "Prod-West" {
		t.Fatalf("alias = %q", resolver.alias)
	}
}

func TestExecuteWrapsResolutionFailure(t *testing.T) {
	_, err := authstatus.New(resolver{err: errors.New("missing")}, lookup{}).Execute(context.Background(), authstatus.Input{Environment: "production"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.status.resolve" {
		t.Fatalf("error = %#v", err)
	}
}

func TestOutputGoldens(t *testing.T) {
	got, _ := authstatus.New(resolver{target: fixture()}, lookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}).Execute(context.Background(), authstatus.Input{})
	assertGolden(t, got, false, "testdata/output.toon")
	assertGolden(t, got, true, "testdata/output_full.toon")
}

func fixture() authstatus.Target {
	return authstatus.Target{Environment: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameVariable: "PROD_PAT_NAME", PATSecretVariable: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
}
func assertGolden(t *testing.T, value any, full bool, path string) {
	t.Helper()
	var b bytes.Buffer
	if err := output.RenderWithOptions(&b, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, b.Bytes())
	}
}
