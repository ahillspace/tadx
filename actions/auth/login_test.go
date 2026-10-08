package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	authops "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type loginResolver struct {
	target authops.LoginTarget
	err    error
	alias  string
}

func (r *loginResolver) Resolve(_ context.Context, alias string) (authops.LoginTarget, error) {
	r.alias = alias
	return r.target, r.err
}

type loginAuthenticator struct {
	result     authops.LoginAuthentication
	err        error
	target     authops.LoginTarget
	credential authops.LoginCredential
	calls      int
}

func (a *loginAuthenticator) Authenticate(_ context.Context, target authops.LoginTarget, credential authops.LoginCredential) (authops.LoginAuthentication, error) {
	a.calls++
	a.target = target
	a.credential = credential
	return a.result, a.err
}

type loginStore struct {
	result     authops.LoginStoreResult
	err        error
	target     authops.LoginTarget
	credential authops.LoginCredential
	calls      int
}

func (s *loginStore) Store(_ context.Context, target authops.LoginTarget, credential authops.LoginCredential) (authops.LoginStoreResult, error) {
	s.calls++
	s.target = target
	s.credential = credential
	return s.result, s.err
}

func TestExecuteValidatesThenStoresCredential(t *testing.T) {
	target := authops.LoginTarget{Environment: "dev", ServerURL: "https://example.test", SiteContentURL: "site"}
	resolver := &loginResolver{target: target}
	authenticator := &loginAuthenticator{result: authops.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}}
	store := &loginStore{}
	action := authops.NewLogin(resolver, authenticator, store)

	got, err := action.Execute(context.Background(), authops.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.alias != "dev" || authenticator.calls != 1 || store.calls != 1 {
		t.Fatalf("calls: alias=%q authenticate=%d store=%d", resolver.alias, authenticator.calls, store.calls)
	}
	if authenticator.credential.PATName != "name" || store.credential.PATSecret != "secret" {
		t.Fatalf("credential handoff was incorrect")
	}
	if got.Status != "stored" || got.Environment != "dev" || got.CredentialSource != "os_credential_store" || !got.Validated {
		t.Fatalf("output = %#v", got)
	}
	if got.SiteLUID != "site-1" || got.UserLUID != "user-1" {
		t.Fatalf("output identity = %#v", got)
	}
}

func TestExecuteDoesNotStoreWhenValidationFails(t *testing.T) {
	resolver := &loginResolver{target: authops.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}}
	authenticator := &loginAuthenticator{err: errors.New("invalid PAT")}
	store := &loginStore{}

	_, err := authops.NewLogin(resolver, authenticator, store).Execute(context.Background(), authops.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	if err == nil || store.calls != 0 {
		t.Fatalf("error = %v, store calls = %d", err, store.calls)
	}
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.authenticate" || !strings.Contains(payload.CorrectiveAction, "No credential was saved") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteReportsValidatedButUnstoredCredential(t *testing.T) {
	action := authops.NewLogin(
		&loginResolver{target: authops.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}},
		&loginAuthenticator{result: authops.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&loginStore{err: errors.New("credential store unavailable")},
	)
	_, err := action.Execute(context.Background(), authops.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.store" || !strings.Contains(payload.Summary, "validated") || !strings.Contains(payload.CorrectiveAction, "not saved") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteRequiresCompletePATInput(t *testing.T) {
	valid := authops.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"}
	tests := []authops.LoginInput{
		{Environment: valid.Environment, PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATName: valid.PATName},
		{Environment: valid.Environment, PATName: "   ", PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATName: valid.PATName, PATSecret: "   "},
	}
	for _, input := range tests {
		_, err := authops.NewLogin(&loginResolver{}, &loginAuthenticator{}, &loginStore{}).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
}

func TestOutputNeverContainsCredentialValues(t *testing.T) {
	action := authops.NewLogin(
		&loginResolver{target: authops.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}},
		&loginAuthenticator{result: authops.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&loginStore{result: authops.LoginStoreResult{EnvironmentVariablesOverride: true}},
	)
	got, err := action.Execute(context.Background(), authops.LoginInput{Environment: "dev", PATName: "private-name", PATSecret: "private-secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		var rendered bytes.Buffer
		if err := output.RenderWithOptions(&rendered, got, output.Options{Full: full}); err != nil {
			t.Fatal(err)
		}
		text := rendered.String()
		if strings.Contains(text, "private-name") || strings.Contains(text, "private-secret") {
			t.Fatalf("credential leaked: %s", text)
		}
		if !strings.Contains(text, "environment variables currently override") {
			t.Fatalf("shadow warning missing: %s", text)
		}
	}
}

func TestCredentialInputsRedactFormattingAndJSON(t *testing.T) {
	input := authops.LoginInput{Environment: "dev", PATName: "private-name", PATSecret: "private-secret"}
	credential := authops.LoginCredential{PATName: input.PATName, PATSecret: input.PATSecret}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	rendered := fmt.Sprintf("%v %+v %#v %v %+v %#v %s %s", input, input, input, credential, credential, credential, inputJSON, credentialJSON)
	if strings.Contains(rendered, input.PATName) || strings.Contains(rendered, input.PATSecret) {
		t.Fatalf("credential formatting leaked a value: %s", rendered)
	}
}

type loginInstalledConfigurationError struct{}

func (loginInstalledConfigurationError) Error() string {
	return "configuration took effect but was not synced"
}
func (loginInstalledConfigurationError) ConfigurationInstalled() bool { return true }

func TestExecuteReportsSavedCredentialWhenConfigurationIsNotDurable(t *testing.T) {
	action := authops.NewLogin(
		&loginResolver{target: authops.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}},
		&loginAuthenticator{result: authops.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&loginStore{err: fmt.Errorf("install configuration: %w", loginInstalledConfigurationError{})},
	)
	_, err := action.Execute(context.Background(), authops.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.store" || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary+payload.CorrectiveAction, "not saved") {
		t.Fatalf("error = %#v", payload)
	}
}
