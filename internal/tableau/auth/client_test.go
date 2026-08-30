package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
)

func TestClientSignsInWithPATJSONWithoutLeakingSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/auth/signin" || request.Method != http.MethodPost {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		credentials := body["credentials"].(map[string]any)
		if credentials["personalAccessTokenName"] != "pat-name" || credentials["personalAccessTokenSecret"] != "pat-secret" {
			t.Fatalf("credentials = %#v", credentials)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"credentials":{"token":"session-secret","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	}))
	defer server.Close()

	client := tableauauth.NewClient(tableau.NewTransport(server.Client(), "3.29", nil))
	response, err := client.SignIn(context.Background(), coreauth.SignInRequest{
		ServerURL: server.URL, SiteContentURL: "marketing", PATName: "pat-name", PATSecret: "pat-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.SiteLUID != "site-1" || response.UserLUID != "user-1" || response.Token != "session-secret" {
		t.Fatalf("response = %#v", response)
	}
	if strings.Contains(response.String(), "session-secret") {
		t.Fatalf("response string leaked token: %s", response.String())
	}
}

func TestClientRedactsPATFromUpstreamErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"code":"401001","summary":"Login error","detail":"bad pat-secret"}}`)
	}))
	defer server.Close()

	client := tableauauth.NewClient(tableau.NewTransport(server.Client(), "3.29", nil))
	_, err := client.SignIn(context.Background(), coreauth.SignInRequest{ServerURL: server.URL, PATName: "pat-name", PATSecret: "pat-secret"})
	if err == nil || strings.Contains(err.Error(), "pat-secret") {
		t.Fatalf("error = %v", err)
	}
}
