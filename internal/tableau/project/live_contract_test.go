//go:build live

package project_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

func TestLiveProjectContract(t *testing.T) {
	environment, session, transport := liveProjectConnection(t)
	client := tableauproject.NewClient(transport, session, environment.URL)
	page, err := client.List(context.Background(), tableauproject.ListRequest{PageNumber: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 1 || page.Size <= 0 || page.Total < len(page.Items) {
		t.Fatalf("invalid live project pagination: %#v", page)
	}
	for _, item := range page.Items {
		if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" {
			t.Fatal("live project response omitted authoritative identity")
		}
	}
	t.Logf("sanitized project evidence: method=GET path=/api/{version}/sites/{site}/projects page=1 size=%d total=%d returned=%d first_luid=%s", page.Size, page.Total, len(page.Items), hashedProjectID(page.Items))
}

func liveProjectConnection(t *testing.T) (config.Environment, coreauth.Session, *tableau.Transport) {
	t.Helper()
	path := strings.TrimSpace(os.Getenv("TADX_LIVE_CONFIG"))
	if path == "" {
		var err error
		path, err = config.UserConfigPath()
		if err != nil {
			t.Fatal(err)
		}
	}
	configuration, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	alias := strings.TrimSpace(os.Getenv("TADX_LIVE_ENVIRONMENT"))
	if alias == "" {
		alias = "dev"
	}
	environment, err := configuration.ResolveEnvironment(alias)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := os.LookupEnv(environment.Auth.PATNameEnv); !ok {
		t.Skip("configured dev PAT variables are not available in this process")
	}
	if _, ok := os.LookupEnv(environment.Auth.PATSecretEnv); !ok {
		t.Skip("configured dev PAT variables are not available in this process")
	}
	transport := tableau.NewTransport(http.DefaultClient, environment.APIVersion, func() string { return "project-live-contract" })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(context.Background(), coreauth.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv})
	if err != nil {
		t.Fatal(err)
	}
	return environment, session, transport
}
func hashedProjectID(items []tableauproject.Project) string {
	if len(items) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(items[0].LUID))
	return fmt.Sprintf("sha256:%x", sum[:8])
}
