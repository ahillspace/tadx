package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	login "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func newLogin(resolver login.LoginResolver, authenticator login.LoginAuthenticator, store login.LoginStore) *login.Service {
	return login.New(login.Ports{LoginResolver: resolver, LoginAuthenticator: authenticator, LoginStore: store, StatusLookup: statusLookup{}})
}

type loginResolver struct {
	target login.LoginTarget
	err    error
	alias  string
}

func (r *loginResolver) Resolve(_ context.Context, alias string) (login.LoginTarget, error) {
	r.alias = alias
	return r.target, r.err
}

type loginAuthenticatorFake struct {
	result     login.LoginAuthentication
	err        error
	target     login.LoginTarget
	credential login.LoginCredential
	calls      int
}

func (a *loginAuthenticatorFake) Authenticate(_ context.Context, target login.LoginTarget, credential login.LoginCredential) (login.LoginAuthentication, error) {
	a.calls++
	a.target = target
	a.credential = credential
	return a.result, a.err
}

type loginStoreFake struct {
	result     login.LoginStoreResult
	err        error
	target     login.LoginTarget
	credential login.LoginCredential
	calls      int
}

func (s *loginStoreFake) Store(_ context.Context, target login.LoginTarget, credential login.LoginCredential) (login.LoginStoreResult, error) {
	s.calls++
	s.target = target
	s.credential = credential
	return s.result, s.err
}

func TestLoginExecuteValidatesThenStoresCredential(t *testing.T) {
	target := login.LoginTarget{Environment: "dev", ServerURL: "https://example.test", SiteContentURL: "site", APIVersion: "3.29"}
	loginResolver := &loginResolver{target: target}
	loginAuthenticatorFake := &loginAuthenticatorFake{result: login.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}}
	loginStoreFake := &loginStoreFake{}
	action := newLogin(loginResolver, loginAuthenticatorFake, loginStoreFake)

	got, err := action.Login(context.Background(), login.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if loginResolver.alias != "dev" || loginAuthenticatorFake.calls != 1 || loginStoreFake.calls != 1 {
		t.Fatalf("calls: alias=%q authenticate=%d store=%d", loginResolver.alias, loginAuthenticatorFake.calls, loginStoreFake.calls)
	}
	if loginAuthenticatorFake.credential.PATName != "name" || loginStoreFake.credential.PATSecret != "secret" {
		t.Fatalf("credential handoff was incorrect")
	}
	if got.Status != "stored" || got.Environment != "dev" || got.CredentialSource != "os_credential_store" || !got.Validated {
		t.Fatalf("output = %#v", got)
	}
	if got.SiteLUID != "site-1" || got.UserLUID != "user-1" {
		t.Fatalf("output identity = %#v", got)
	}
}

func TestLoginExecuteDoesNotStoreWhenValidationFails(t *testing.T) {
	loginResolver := &loginResolver{target: login.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}}
	loginAuthenticatorFake := &loginAuthenticatorFake{err: errors.New("invalid PAT")}
	loginStoreFake := &loginStoreFake{}

	_, err := newLogin(loginResolver, loginAuthenticatorFake, loginStoreFake).Login(context.Background(), login.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	if err == nil || loginStoreFake.calls != 0 {
		t.Fatalf("error = %v, store calls = %d", err, loginStoreFake.calls)
	}
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.authenticate" || !strings.Contains(payload.CorrectiveAction, "No credential was saved") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestLoginExecuteReportsValidatedButUnstoredCredential(t *testing.T) {
	action := newLogin(
		&loginResolver{target: login.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}},
		&loginAuthenticatorFake{result: login.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&loginStoreFake{err: errors.New("credential store unavailable")},
	)
	_, err := action.Login(context.Background(), login.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.store" || !strings.Contains(payload.Summary, "validated") || !strings.Contains(payload.CorrectiveAction, "not saved") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestLoginExecuteRequiresCompletePATInput(t *testing.T) {
	valid := login.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"}
	tests := []login.LoginInput{
		{Environment: valid.Environment, PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATName: valid.PATName},
		{Environment: valid.Environment, PATName: "   ", PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATName: valid.PATName, PATSecret: "   "},
	}
	for _, input := range tests {
		_, err := newLogin(&loginResolver{}, &loginAuthenticatorFake{}, &loginStoreFake{}).Login(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
}

func TestLoginOutputNeverContainsCredentialValues(t *testing.T) {
	action := newLogin(
		&loginResolver{target: login.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}},
		&loginAuthenticatorFake{result: login.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&loginStoreFake{result: login.LoginStoreResult{EnvironmentVariablesOverride: true}},
	)
	got, err := action.Login(context.Background(), login.LoginInput{Environment: "dev", PATName: "private-name", PATSecret: "private-secret"})
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

func TestLoginCredentialInputsRedactFormattingAndJSON(t *testing.T) {
	input := login.LoginInput{Environment: "dev", PATName: "private-name", PATSecret: "private-secret"}
	credential := login.LoginCredential{PATName: input.PATName, PATSecret: input.PATSecret}
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

func TestLoginExecuteReportsSavedCredentialWhenConfigurationIsNotDurable(t *testing.T) {
	action := newLogin(
		&loginResolver{target: login.LoginTarget{Environment: "dev", ServerURL: "https://example.test"}},
		&loginAuthenticatorFake{result: login.LoginAuthentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&loginStoreFake{err: fmt.Errorf("install configuration: %w", loginInstalledConfigurationError{})},
	)
	_, err := action.Login(context.Background(), login.LoginInput{Environment: "dev", PATName: "name", PATSecret: "secret"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.store" || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary+payload.CorrectiveAction, "not saved") {
		t.Fatalf("error = %#v", payload)
	}
}
