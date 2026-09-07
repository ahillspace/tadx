package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

type fixedCredentialPrompter struct {
	name   string
	secret string
}

func (fixedCredentialPrompter) IsTerminal() bool { return true }
func (p fixedCredentialPrompter) ReadPATName(context.Context) (string, error) {
	return p.name, nil
}
func (p fixedCredentialPrompter) ReadPATSecret(context.Context) (string, error) {
	return p.secret, nil
}

func TestAuthLoginCheckAndLogoutThroughCLI(t *testing.T) {
	t.Setenv("TADX_DEV_PAT_NAME", "")
	t.Setenv("TADX_DEV_PAT_SECRET", "")
	const patName = "interactive-name"
	const patSecret = "interactive-secret"
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/auth/signin" {
			http.NotFound(writer, request)
			return
		}
		body, _ := io.ReadAll(request.Body)
		if !bytes.Contains(body, []byte(patName)) || !bytes.Contains(body, []byte(patSecret)) {
			t.Error("sign-in request did not contain the prompted PAT")
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	}))
	defer server.Close()

	path := authConfig(t, "")
	configuration, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	environment := configuration.Environments["dev"]
	environment.URL = server.URL
	configuration.Environments["dev"] = environment
	if err := config.Save(path, configuration); err != nil {
		t.Fatal(err)
	}
	store := &fakePATStore{next: coreauth.CredentialReference("cred_33333333333333333333333333333333")}
	options := Options{ConfigPath: path, HTTPClient: server.Client(), PATStore: store, AuthPrompter: fixedCredentialPrompter{name: patName, secret: patSecret}}

	var output bytes.Buffer
	if code := Run(context.Background(), []string{"auth", "login", "--environment", "dev"}, &output, options); code != 0 {
		t.Fatalf("login exit = %d, output = %s", code, output.String())
	}
	if strings.Contains(output.String(), patName) || strings.Contains(output.String(), patSecret) || !strings.Contains(output.String(), "credential_source: os_credential_store") {
		t.Fatalf("unsafe or incomplete login output: %s", output.String())
	}

	output.Reset()
	if code := Run(context.Background(), []string{"auth", "status", "--environment", "dev"}, &output, options); code != 0 || !strings.Contains(output.String(), "stored_credential_reference_present: true") || !strings.Contains(output.String(), "credential_source: os_credential_store") {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := Run(context.Background(), []string{"auth", "check", "--environment", "dev"}, &output, options); code != 0 || !strings.Contains(output.String(), "status: authenticated") {
		t.Fatalf("check exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := Run(context.Background(), []string{"auth", "logout", "--environment", "dev"}, &output, options); code != 0 {
		t.Fatalf("logout exit = %d, output = %s", code, output.String())
	}
	if !strings.Contains(output.String(), "tableau_pat_revoked: false") {
		t.Fatalf("logout output = %s", output.String())
	}
}
