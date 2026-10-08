package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	authops "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type statusResolver struct {
	target authops.StatusTarget
	err    error
}

func (r statusResolver) Resolve(context.Context, string) (authops.StatusTarget, error) {
	return r.target, r.err
}

type recordingResolver struct {
	target authops.StatusTarget
	alias  string
}

func (r *recordingResolver) Resolve(_ context.Context, alias string) (authops.StatusTarget, error) {
	r.alias = alias
	return r.target, nil
}

type lookup map[string]string

func (l lookup) LookupEnv(key string) (string, bool) { value, ok := l[key]; return value, ok }

func TestExecuteReportsReadyWithoutReturningPATValues(t *testing.T) {
	action := authops.NewStatus(statusResolver{target: fixture()}, lookup{"PROD_PAT_NAME": "token-name", "PROD_PAT_SECRET": "highly-secret"})
	got, err := action.Execute(context.Background(), authops.StatusInput{Environment: "production"})
	if err != nil || got.Status != "ready" || got.CredentialSource != "environment" || !got.PATNamePresent || !got.PATSecretPresent {
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

func TestExecuteReportsStoredCredentialWithoutReadingItsValues(t *testing.T) {
	target := fixture()
	target.StoredCredentialReferencePresent = true
	got, err := authops.NewStatus(statusResolver{target: target}, lookup{}).Execute(context.Background(), authops.StatusInput{})
	if err != nil || got.Status != "ready" || got.CredentialSource != "os_credential_store" || !got.StoredCredentialReferencePresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecuteReportsEnvironmentOverrideOfStoredCredential(t *testing.T) {
	target := fixture()
	target.StoredCredentialReferencePresent = true
	got, err := authops.NewStatus(statusResolver{target: target}, lookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}).Execute(context.Background(), authops.StatusInput{})
	if err != nil || got.CredentialSource != "environment" || !got.StoredCredentialReferencePresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecuteReportsIncompleteForMissingOrEmptyValues(t *testing.T) {
	got, err := authops.NewStatus(statusResolver{target: fixture()}, lookup{"PROD_PAT_NAME": "  "}).Execute(context.Background(), authops.StatusInput{})
	if err != nil || got.Status != "incomplete" || got.PATNamePresent || got.PATSecretPresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecutePreservesExactAlias(t *testing.T) {
	resolver := &recordingResolver{target: fixture()}
	_, err := authops.NewStatus(resolver, lookup{}).Execute(context.Background(), authops.StatusInput{Environment: "Prod-West"})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.alias != "Prod-West" {
		t.Fatalf("alias = %q", resolver.alias)
	}
}

func TestExecuteWrapsResolutionFailure(t *testing.T) {
	_, err := authops.NewStatus(statusResolver{err: errors.New("missing")}, lookup{}).Execute(context.Background(), authops.StatusInput{Environment: "production"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.status.resolve" {
		t.Fatalf("error = %#v", err)
	}
}

func TestOutputGoldens(t *testing.T) {
	got, _ := authops.NewStatus(statusResolver{target: fixture()}, lookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}).Execute(context.Background(), authops.StatusInput{})
	assertGolden(t, got, false, "testdata/status/output.toon")
	assertGolden(t, got, true, "testdata/status/output_full.toon")
}

func TestInspectAndFullOutputKeepTheResolvedContract(t *testing.T) {
	target := fixture()
	values := lookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}
	fromAction, err := authops.NewStatus(statusResolver{target: target}, values).Execute(t.Context(), authops.StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if fromInspect := authops.InspectStatus(target, values); fromInspect.Status != fromAction.Status || fromInspect.CredentialSource != fromAction.CredentialSource {
		t.Fatalf("local readiness drift: action = %#v, inspect = %#v", fromAction, fromInspect)
	}
	full, err := json.Marshal(fromAction.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(fromAction)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(full, canonical) {
		t.Fatalf("full output differs from canonical output: %s versus %s", full, canonical)
	}
}

func fixture() authops.StatusTarget {
	return authops.StatusTarget{Environment: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", AuthType: "pat", PATNameVariable: "PROD_PAT_NAME", PATSecretVariable: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
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
