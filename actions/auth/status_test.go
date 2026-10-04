package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	authstatus "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func newStatus(resolver authstatus.StatusResolver, lookup authstatus.StatusLookupEnv) *authstatus.Service {
	return authstatus.New(authstatus.Ports{StatusResolver: resolver, StatusLookup: lookup})
}

type statusResolver struct {
	target authstatus.StatusTarget
	err    error
}

func (r statusResolver) Resolve(context.Context, string) (authstatus.StatusTarget, error) {
	return r.target, r.err
}

type statusRecordingResolver struct {
	target authstatus.StatusTarget
	alias  string
}

func (r *statusRecordingResolver) Resolve(_ context.Context, alias string) (authstatus.StatusTarget, error) {
	r.alias = alias
	return r.target, nil
}

type statusLookup map[string]string

func (l statusLookup) LookupEnv(key string) (string, bool) { value, ok := l[key]; return value, ok }

func TestStatusExecuteReportsReadyWithoutReturningPATValues(t *testing.T) {
	action := newStatus(statusResolver{target: statusFixture()}, statusLookup{"PROD_PAT_NAME": "token-name", "PROD_PAT_SECRET": "highly-secret"})
	got, err := action.Status(context.Background(), authstatus.StatusInput{Environment: "production"})
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

func TestStatusExecuteReportsStoredCredentialWithoutReadingItsValues(t *testing.T) {
	target := statusFixture()
	target.StoredCredentialReferencePresent = true
	got, err := newStatus(statusResolver{target: target}, statusLookup{}).Status(context.Background(), authstatus.StatusInput{})
	if err != nil || got.Status != "ready" || got.CredentialSource != "os_credential_store" || !got.StoredCredentialReferencePresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestStatusExecuteReportsEnvironmentOverrideOfStoredCredential(t *testing.T) {
	target := statusFixture()
	target.StoredCredentialReferencePresent = true
	got, err := newStatus(statusResolver{target: target}, statusLookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}).Status(context.Background(), authstatus.StatusInput{})
	if err != nil || got.CredentialSource != "environment" || !got.StoredCredentialReferencePresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestStatusExecuteReportsIncompleteForMissingOrEmptyValues(t *testing.T) {
	got, err := newStatus(statusResolver{target: statusFixture()}, statusLookup{"PROD_PAT_NAME": "  "}).Status(context.Background(), authstatus.StatusInput{})
	if err != nil || got.Status != "incomplete" || got.PATNamePresent || got.PATSecretPresent {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestStatusExecutePreservesExactAlias(t *testing.T) {
	statusResolver := &statusRecordingResolver{target: statusFixture()}
	_, err := newStatus(statusResolver, statusLookup{}).Status(context.Background(), authstatus.StatusInput{Environment: "Prod-West"})
	if err != nil {
		t.Fatal(err)
	}
	if statusResolver.alias != "Prod-West" {
		t.Fatalf("alias = %q", statusResolver.alias)
	}
}

func TestStatusExecuteWrapsResolutionFailure(t *testing.T) {
	_, err := newStatus(statusResolver{err: errors.New("missing")}, statusLookup{}).Status(context.Background(), authstatus.StatusInput{Environment: "production"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.status.resolve" {
		t.Fatalf("error = %#v", err)
	}
}

func TestStatusOutputGoldens(t *testing.T) {
	got, _ := newStatus(statusResolver{target: statusFixture()}, statusLookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}).Status(context.Background(), authstatus.StatusInput{})
	assertGolden(t, got, false, "testdata/status/output.toon")
	assertGolden(t, got, true, "testdata/status/output_full.toon")
}

func TestStatusInspectAndFullOutputKeepTheResolvedContract(t *testing.T) {
	target := statusFixture()
	values := statusLookup{"PROD_PAT_NAME": "name", "PROD_PAT_SECRET": "secret"}
	fromAction, err := newStatus(statusResolver{target: target}, values).Status(t.Context(), authstatus.StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if fromInspect := authstatus.InspectStatus(target, values); fromInspect.Status != fromAction.Status || fromInspect.CredentialSource != fromAction.CredentialSource {
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

func statusFixture() authstatus.StatusTarget {
	return authstatus.StatusTarget{Environment: "production", Default: true, ServerURL: "https://example.test", SiteContentURL: "marketing", APIVersion: "3.29", AuthType: "pat", PATNameVariable: "PROD_PAT_NAME", PATSecretVariable: "PROD_PAT_SECRET", DefaultWorkspace: "primary"}
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
