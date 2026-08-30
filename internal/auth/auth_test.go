package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/auth"
)

func TestPATProviderResolvesVariablesAndReturnsAuthenticatedSession(t *testing.T) {
	t.Parallel()

	lookup := auth.LookupEnvFunc(func(key string) (string, bool) {
		values := map[string]string{
			"PAT_NAME":   "agent-name",
			"PAT_SECRET": "highly-secret",
		}
		value, ok := values[key]
		return value, ok
	})
	signer := &recordingSigner{response: auth.SignInResponse{
		Token:    "session-token",
		SiteLUID: "site-luid",
		UserLUID: "user-luid",
	}}
	provider := auth.NewPATProvider(lookup, signer)

	session, err := provider.Authenticate(context.Background(), auth.Target{
		Environment:       "production",
		ServerURL:         "https://example.tableau.com",
		SiteContentURL:    "example-site",
		PATNameVariable:   "PAT_NAME",
		PATSecretVariable: "PAT_SECRET",
	})
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if signer.request.PATName != "agent-name" || signer.request.PATSecret != "highly-secret" {
		t.Fatalf("sign-in credentials = %#v", signer.request)
	}
	if session.SiteLUID() != "site-luid" || session.UserLUID() != "user-luid" {
		t.Fatalf("session identity = %q, %q", session.SiteLUID(), session.UserLUID())
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	session.Authorize(req)
	if got := req.Header.Get(auth.TableauAuthHeader); got != "session-token" {
		t.Fatalf("auth header = %q", got)
	}
	if got := session.String(); got != "authenticated Tableau session for site site-luid" {
		t.Fatalf("session String() = %q", got)
	}
}

func TestProviderSeamDoesNotExposePATCredentials(t *testing.T) {
	t.Parallel()

	providerType := reflect.TypeFor[auth.Provider]()
	method, ok := providerType.MethodByName("Authenticate")
	if !ok {
		t.Fatal("Provider has no Authenticate method")
	}
	for i := 0; i < method.Type.NumOut(); i++ {
		if method.Type.Out(i).Name() == "SignInRequest" {
			t.Fatal("Provider exposes PAT credentials")
		}
	}
}

func TestSignInTypesRedactCredentialAndTokenFormatting(t *testing.T) {
	t.Parallel()

	request := auth.SignInRequest{PATName: "agent-name", PATSecret: "highly-secret"}
	response := auth.SignInResponse{Token: "session-token", SiteLUID: "site-luid"}
	formatted := fmt.Sprintf("%v %+v %#v %v %+v %#v", request, request, request, response, response, response)
	requestJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	responseJSON, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	formatted += string(requestJSON) + string(responseJSON)
	for _, secret := range []string{"agent-name", "highly-secret", "session-token"} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("formatted auth values contain %q", secret)
		}
	}
}

func TestPATProviderReportsMissingVariablesWithoutRevealingValues(t *testing.T) {
	t.Parallel()

	provider := auth.NewPATProvider(auth.LookupEnvFunc(func(string) (string, bool) {
		return "", false
	}), &recordingSigner{})

	_, err := provider.Authenticate(context.Background(), auth.Target{
		Environment:       "production",
		ServerURL:         "https://example.tableau.com",
		PATNameVariable:   "PAT_NAME",
		PATSecretVariable: "PAT_SECRET",
	})
	if err == nil {
		t.Fatal("Authenticate() error = nil")
	}
	var missing *auth.MissingVariablesError
	if !errors.As(err, &missing) {
		t.Fatalf("error type = %T", err)
	}
	if got, want := missing.Variables, []string{"PAT_NAME", "PAT_SECRET"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Variables = %#v, want %#v", got, want)
	}
}

func TestPATProviderRejectsEmptyVariableValues(t *testing.T) {
	t.Parallel()

	provider := auth.NewPATProvider(auth.LookupEnvFunc(func(string) (string, bool) {
		return "", true
	}), &recordingSigner{})
	_, err := provider.Authenticate(context.Background(), auth.Target{
		PATNameVariable:   "PAT_NAME",
		PATSecretVariable: "PAT_SECRET",
	})
	if err == nil {
		t.Fatal("Authenticate() error = nil")
	}
}

func TestPATProviderRedactsCredentialsFromSignInErrors(t *testing.T) {
	t.Parallel()

	provider := auth.NewPATProvider(auth.LookupEnvFunc(func(key string) (string, bool) {
		return map[string]string{"PAT_NAME": "agent-name", "PAT_SECRET": "highly-secret"}[key], true
	}), &recordingSigner{err: errors.New("upstream rejected agent-name with highly-secret")})
	_, err := provider.Authenticate(context.Background(), auth.Target{
		PATNameVariable:   "PAT_NAME",
		PATSecretVariable: "PAT_SECRET",
	})
	if err == nil {
		t.Fatal("Authenticate() error = nil")
	}
	if got, want := err.Error(), "sign in: upstream rejected [REDACTED] with [REDACTED]"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

type recordingSigner struct {
	request  auth.SignInRequest
	response auth.SignInResponse
	err      error
}

func (s *recordingSigner) SignIn(_ context.Context, request auth.SignInRequest) (auth.SignInResponse, error) {
	s.request = request
	return s.response, s.err
}
