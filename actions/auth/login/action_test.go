package login_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	login "github.com/ahillspace/tadx/actions/auth/login"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	target login.Target
	err    error
	alias  string
}

func (r *resolver) Resolve(_ context.Context, alias string) (login.Target, error) {
	r.alias = alias
	return r.target, r.err
}

type authenticator struct {
	result     login.Authentication
	err        error
	target     login.Target
	credential login.Credential
	calls      int
}

func (a *authenticator) Authenticate(_ context.Context, target login.Target, credential login.Credential) (login.Authentication, error) {
	a.calls++
	a.target = target
	a.credential = credential
	return a.result, a.err
}

type store struct {
	result     login.StoreResult
	err        error
	target     login.Target
	credential login.Credential
	calls      int
}

func (s *store) Store(_ context.Context, target login.Target, credential login.Credential) (login.StoreResult, error) {
	s.calls++
	s.target = target
	s.credential = credential
	return s.result, s.err
}

func TestExecuteValidatesThenStoresCredential(t *testing.T) {
	target := login.Target{Environment: "dev", ServerURL: "https://example.test", SiteContentURL: "site", APIVersion: "3.29"}
	resolver := &resolver{target: target}
	authenticator := &authenticator{result: login.Authentication{SiteLUID: "site-1", UserLUID: "user-1"}}
	store := &store{}
	action := login.New(resolver, authenticator, store)

	got, err := action.Execute(context.Background(), login.Input{Environment: "dev", PATName: "name", PATSecret: "secret"})
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
	resolver := &resolver{target: login.Target{Environment: "dev", ServerURL: "https://example.test"}}
	authenticator := &authenticator{err: errors.New("invalid PAT")}
	store := &store{}

	_, err := login.New(resolver, authenticator, store).Execute(context.Background(), login.Input{Environment: "dev", PATName: "name", PATSecret: "secret"})
	if err == nil || store.calls != 0 {
		t.Fatalf("error = %v, store calls = %d", err, store.calls)
	}
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.authenticate" || !strings.Contains(payload.CorrectiveAction, "No credential was saved") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteReportsValidatedButUnstoredCredential(t *testing.T) {
	action := login.New(
		&resolver{target: login.Target{Environment: "dev", ServerURL: "https://example.test"}},
		&authenticator{result: login.Authentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&store{err: errors.New("credential store unavailable")},
	)
	_, err := action.Execute(context.Background(), login.Input{Environment: "dev", PATName: "name", PATSecret: "secret"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.login.store" || !strings.Contains(payload.Summary, "validated") || !strings.Contains(payload.CorrectiveAction, "not saved") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteRequiresExplicitCompleteInput(t *testing.T) {
	valid := login.Input{Environment: "dev", PATName: "name", PATSecret: "secret"}
	tests := []login.Input{
		{PATName: valid.PATName, PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATName: valid.PATName},
		{Environment: valid.Environment, PATName: "   ", PATSecret: valid.PATSecret},
		{Environment: valid.Environment, PATName: valid.PATName, PATSecret: "   "},
	}
	for _, input := range tests {
		_, err := login.New(&resolver{}, &authenticator{}, &store{}).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
			t.Fatalf("input %#v error = %#v", input, err)
		}
	}
}

func TestOutputNeverContainsCredentialValues(t *testing.T) {
	action := login.New(
		&resolver{target: login.Target{Environment: "dev", ServerURL: "https://example.test"}},
		&authenticator{result: login.Authentication{SiteLUID: "site-1", UserLUID: "user-1"}},
		&store{result: login.StoreResult{EnvironmentVariablesOverride: true}},
	)
	got, err := action.Execute(context.Background(), login.Input{Environment: "dev", PATName: "private-name", PATSecret: "private-secret"})
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
	input := login.Input{Environment: "dev", PATName: "private-name", PATSecret: "private-secret"}
	credential := login.Credential{PATName: input.PATName, PATSecret: input.PATSecret}
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
