package app

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

type warningTestNoNetwork struct{ calls int }

func (transport *warningTestNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	transport.calls++
	return nil, errors.New("network is forbidden in local warning tests")
}

func TestSecondEnvironmentAddWarnsAboutExplicitWriteTargetThroughCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	transport := &warningTestNoNetwork{}
	options := Options{ConfigPath: path, HTTPClient: &http.Client{Transport: transport}, PATStore: &fakePATStore{}}
	for index, alias := range []string{"first", "second"} {
		var out strings.Builder
		exit := Run(context.Background(), []string{"env", "add", alias, "--url", "https://tableau.example.test", "--site", alias}, &out, options)
		if exit != 0 {
			t.Fatalf("add %s exit=%d output=%s", alias, exit, out.String())
		}
		if index == 0 && strings.Contains(out.String(), "warnings[") {
			t.Fatalf("first environment unexpectedly warned: %s", out.String())
		}
		if index == 1 && (!strings.Contains(out.String(), "warnings[") || !strings.Contains(out.String(), "Multiple environments") || !strings.Contains(out.String(), "Remote writes require --env <name>")) {
			t.Fatalf("second environment omitted target warning: %s", out.String())
		}
	}
	stored, err := config.Load(path)
	if err != nil || len(stored.Environments) != 2 || transport.calls != 0 {
		t.Fatalf("environments=%d requests=%d err=%v", len(stored.Environments), transport.calls, err)
	}
}

func TestLogoutWarnsWhenEnvironmentPATRemainsUsableThroughCLI(t *testing.T) {
	const nameKey, secretKey = "TADX_WARNING_TEST_PAT_NAME", "TADX_WARNING_TEST_PAT_SECRET"
	const environmentName, environmentSecret = "fixture-env-name", "fixture-env-secret"
	for _, available := range []bool{false, true} {
		label := "without_environment_pair"
		if available {
			label = "with_environment_pair"
		}
		t.Run(label, func(t *testing.T) {
			t.Setenv(nameKey, environmentName)
			t.Setenv(secretKey, "")
			if available {
				t.Setenv(secretKey, environmentSecret)
			}
			reference := coreauth.CredentialReference("cred_77777777777777777777777777777777")
			path := authConfig(t, string(reference))
			stored, err := config.Load(path)
			if err != nil {
				t.Fatal(err)
			}
			environment := stored.Environments["dev"]
			environment.Auth.PATNameEnv, environment.Auth.PATSecretEnv = nameKey, secretKey
			stored.Environments["dev"] = environment
			if err := config.Save(path, stored); err != nil {
				t.Fatal(err)
			}
			store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-stored-name", Secret: "fixture-stored-secret", Source: coreauth.CredentialSourceOSKeyring}}}
			transport := &warningTestNoNetwork{}
			var out strings.Builder
			exit := Run(context.Background(), []string{"auth", "logout", "--environment", "dev"}, &out, Options{ConfigPath: path, PATStore: store, HTTPClient: &http.Client{Transport: transport}})
			if exit != 0 || !strings.Contains(out.String(), "status: removed") || !strings.Contains(out.String(), "tableau_pat_revoked: false") {
				t.Fatalf("logout exit=%d output=%s", exit, out.String())
			}
			warned := strings.Contains(out.String(), "Commands can still authenticate") && strings.Contains(out.String(), "Environment-variable credentials remain configured")
			if warned != available {
				t.Fatalf("available=%t warning=%t output=%s", available, warned, out.String())
			}
			for _, secret := range []string{environmentName, environmentSecret, "fixture-stored-name", "fixture-stored-secret"} {
				if strings.Contains(out.String(), secret) {
					t.Fatal("logout output exposed a fixture credential")
				}
			}
			stored, err = config.Load(path)
			if err != nil || stored.Environments["dev"].Auth.CredentialRef != "" || stored.Environments["dev"].Auth.PATSecretEnv != secretKey || len(store.deleted) != 1 || transport.calls != 0 {
				t.Fatalf("logout did not remove only stored credentials: requests=%d deletes=%d err=%v", transport.calls, len(store.deleted), err)
			}
		})
	}
}
