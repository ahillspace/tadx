//go:build live

package search_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

func TestLiveNativeSearchContract(t *testing.T) {
	environment, session, transport := liveSearchConnection(t)
	term := strings.TrimSpace(os.Getenv("TADX_LIVE_SEARCH_TERM"))
	if term == "" {
		t.Skip("set TADX_LIVE_SEARCH_TERM")
	}
	client, err := tableausearch.NewClient(transport, session, environment.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.Search(context.Background(), tableausearch.Request{Terms: term, Types: []string{"workbook"}, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) == 0 || page.Total < len(page.Items) || strings.TrimSpace(page.TableauRequestID) == "" {
		t.Fatalf("invalid live search response: returned=%d total=%d request_id_present=%t", len(page.Items), page.Total, page.TableauRequestID != "")
	}
	t.Logf("verified native search: returned=%d total=%d", len(page.Items), page.Total)
}

func liveSearchConnection(t *testing.T) (config.Environment, coreauth.Session, *tableau.Transport) {
	t.Helper()
	path, err := config.UserConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	alias := strings.TrimSpace(os.Getenv("TADX_LIVE_ENVIRONMENT"))
	if alias == "" {
		t.Skip("set TADX_LIVE_ENVIRONMENT")
	}
	environment, err := configuration.ResolveEnvironment(alias)
	if err != nil {
		t.Fatal(err)
	}
	if environment.SiteContentURL != strings.TrimSpace(os.Getenv("TADX_LIVE_SITE_CONTENT_URL")) {
		t.Fatal("live search site guard failed")
	}
	transport := tableau.NewTransport(http.DefaultClient, environment.APIVersion, func() string { return "native-search-live-contract" })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(context.Background(), coreauth.Target{
		Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL,
		PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	return environment, session, transport
}
