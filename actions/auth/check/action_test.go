package check_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	check "github.com/ahillspace/tadx/actions/auth/check"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type environmentResolver struct{ target check.Target }

func (r environmentResolver) Resolve(_ context.Context, _ string) (check.Target, error) {
	return r.target, nil
}

type authenticator struct {
	result check.Authentication
	err    error
}

func (a authenticator) Authenticate(context.Context, check.Target) (check.Authentication, error) {
	return a.result, a.err
}

type retryableAuthError struct{}

func (retryableAuthError) Error() string            { return "Tableau unavailable" }
func (retryableAuthError) Retryable() bool          { return true }
func (retryableAuthError) CorrectiveAction() string { return "Retry after Tableau recovers." }

func TestActionReturnsRedactedAuthenticatedIdentity(t *testing.T) {
	target := check.Target{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET"}
	action := check.New(environmentResolver{target: target}, authenticator{result: check.Authentication{SiteLUID: "site-1", UserLUID: "user-1"}})
	output, err := action.Execute(context.Background(), check.Input{Environment: "production"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(output)
	if output.Status != "authenticated" || output.SiteLUID != "site-1" || strings.Contains(string(data), "PAT_SECRET") {
		t.Fatalf("output = %s", data)
	}
}

func TestActionWrapsAuthenticationFailureWithStableContext(t *testing.T) {
	target := check.Target{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	action := check.New(environmentResolver{target: target}, authenticator{err: errors.New("invalid PAT")})
	_, err := action.Execute(context.Background(), check.Input{Environment: "production"})
	if err == nil || !strings.Contains(err.Error(), "Authentication check failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestActionPreservesAuthenticationRetryAdvice(t *testing.T) {
	t.Parallel()

	target := check.Target{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	action := check.New(environmentResolver{target: target}, authenticator{err: retryableAuthError{}})
	_, err := action.Execute(context.Background(), check.Input{Environment: "production"})
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestActionGoldenOutput(t *testing.T) {
	value := check.Output{
		Status: "authenticated", Environment: "production", ServerURL: "https://example.test",
		SiteContentURL: "marketing", SiteLUID: "site-1", UserLUID: "user-1",
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
