package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	check "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func newCheck(resolver check.CheckEnvironmentResolver, authenticator check.CheckAuthenticator) *check.Service {
	return check.New(check.Ports{CheckResolver: resolver, CheckAuthenticator: authenticator})
}

type checkEnvironmentResolver struct {
	target check.CheckTarget
	err    error
}

func (r checkEnvironmentResolver) Resolve(_ context.Context, _ string) (check.CheckTarget, error) {
	return r.target, r.err
}

type checkAuthenticator struct {
	result check.CheckAuthentication
	err    error
}

func (a checkAuthenticator) Authenticate(context.Context, check.CheckTarget) (check.CheckAuthentication, error) {
	return a.result, a.err
}

type checkRetryableAuthError struct{}

func (checkRetryableAuthError) Error() string            { return "Tableau unavailable" }
func (checkRetryableAuthError) Retryable() bool          { return true }
func (checkRetryableAuthError) CorrectiveAction() string { return "Retry after Tableau recovers." }

func TestCheckActionReturnsRedactedAuthenticatedIdentity(t *testing.T) {
	target := check.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET"}
	action := newCheck(checkEnvironmentResolver{target: target}, checkAuthenticator{result: check.CheckAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}})
	output, err := action.Check(context.Background(), check.CheckInput{Environment: "production"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(output)
	if output.Status != "authenticated" || output.SiteLUID != "site-1" || strings.Contains(string(data), "PAT_SECRET") {
		t.Fatalf("output = %s", data)
	}
}

func TestCheckActionWrapsAuthenticationFailureWithStableContext(t *testing.T) {
	target := check.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	action := newCheck(checkEnvironmentResolver{target: target}, checkAuthenticator{err: errors.New("invalid PAT")})
	_, err := action.Check(context.Background(), check.CheckInput{Environment: "production"})
	if err == nil || !strings.Contains(err.Error(), "Authentication check failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckActionPreservesAuthenticationRetryAdvice(t *testing.T) {
	t.Parallel()

	target := check.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	action := newCheck(checkEnvironmentResolver{target: target}, checkAuthenticator{err: checkRetryableAuthError{}})
	_, err := action.Check(context.Background(), check.CheckInput{Environment: "production"})
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestCheckActionCompletesEnvironmentErrorAdvice(t *testing.T) {
	action := newCheck(checkEnvironmentResolver{err: errors.New("profile missing")}, checkAuthenticator{})
	_, err := action.Check(context.Background(), check.CheckInput{Environment: "production"})
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || *payload.Retryable || payload.CorrectiveAction == "" || payload.Operation != "auth.check" {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestCheckActionGoldenOutput(t *testing.T) {
	target := check.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	value, err := newCheck(checkEnvironmentResolver{target: target}, checkAuthenticator{result: check.CheckAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}}).Check(context.Background(), check.CheckInput{Environment: "production"})
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/check/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
