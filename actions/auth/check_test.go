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

type environmentResolver struct {
	target authops.CheckTarget
	err    error
}

func (r environmentResolver) Resolve(_ context.Context, _ string) (authops.CheckTarget, error) {
	return r.target, r.err
}

type authenticator struct {
	result authops.CheckAuthentication
	err    error
}

func (a authenticator) Authenticate(context.Context, authops.CheckTarget) (authops.CheckAuthentication, error) {
	return a.result, a.err
}

type retryableAuthError struct{}

func (retryableAuthError) Error() string            { return "Tableau unavailable" }
func (retryableAuthError) Retryable() bool          { return true }
func (retryableAuthError) CorrectiveAction() string { return "Retry after Tableau recovers." }

func TestActionReturnsRedactedAuthenticatedIdentity(t *testing.T) {
	target := authops.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET"}
	action := authops.NewCheck(environmentResolver{target: target}, authenticator{result: authops.CheckAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}})
	output, err := action.Execute(context.Background(), authops.CheckInput{Environment: "production"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(output)
	if output.Status != "authenticated" || output.SiteLUID != "site-1" || strings.Contains(string(data), "PAT_SECRET") {
		t.Fatalf("output = %s", data)
	}
}

func TestActionWrapsAuthenticationFailureWithStableContext(t *testing.T) {
	target := authops.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	action := authops.NewCheck(environmentResolver{target: target}, authenticator{err: errors.New("invalid PAT")})
	_, err := action.Execute(context.Background(), authops.CheckInput{Environment: "production"})
	if err == nil || !strings.Contains(err.Error(), "Authentication check failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestActionPreservesAuthenticationRetryAdvice(t *testing.T) {
	t.Parallel()

	target := authops.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	action := authops.NewCheck(environmentResolver{target: target}, authenticator{err: retryableAuthError{}})
	_, err := action.Execute(context.Background(), authops.CheckInput{Environment: "production"})
	if err == nil {
		t.Fatal("Execute() error = nil")
	}
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestActionCompletesEnvironmentErrorAdvice(t *testing.T) {
	action := authops.NewCheck(environmentResolver{err: errors.New("profile missing")}, authenticator{})
	_, err := action.Execute(context.Background(), authops.CheckInput{Environment: "production"})
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || *payload.Retryable || payload.CorrectiveAction == "" || payload.Operation != "auth.check" {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestActionGoldenOutput(t *testing.T) {
	target := authops.CheckTarget{Environment: "production", ServerURL: "https://example.test", SiteContentURL: "marketing"}
	value, err := authops.NewCheck(environmentResolver{target: target}, authenticator{result: authops.CheckAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}}).Execute(context.Background(), authops.CheckInput{Environment: "production"})
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
