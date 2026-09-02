//go:build live

package workbook_test

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
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

func TestLiveWorkbookInventoryContract(t *testing.T) {
	workbookLUID := strings.TrimSpace(os.Getenv("TADX_LIVE_WORKBOOK_LUID"))
	if workbookLUID == "" {
		t.Skip("set TADX_LIVE_WORKBOOK_LUID to run the live workbook inventory contract test")
	}
	environment, session, transport := liveWorkbookConnection(t)
	client := tableauworkbook.NewClient(transport, session, environment.URL)
	item, err := client.Get(context.Background(), workbookLUID)
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != workbookLUID || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ProjectLUID) == "" || strings.TrimSpace(item.TableauRequestID) == "" {
		t.Fatalf("invalid exact live workbook response: luid=%s name_present=%t project_present=%t request_id_present=%t", hashedWorkbookValue(item.LUID), item.Name != "", item.ProjectLUID != "", item.TableauRequestID != "")
	}
	page, err := client.ListWorkbooks(context.Background(), tableauworkbook.ListRequest{PageNumber: 1, PageSize: 25, Name: item.Name, ProjectName: item.ProjectName})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, candidate := range page.Items {
		if candidate.LUID == workbookLUID {
			found = true
			break
		}
	}
	if page.Page.Number != 1 || page.Page.Size < 1 || page.Page.Size > 25 || page.Page.Total < len(page.Items) || strings.TrimSpace(page.TableauRequestID) == "" || !found {
		t.Fatalf("invalid filtered live workbook page: number=%d size=%d total=%d returned=%d request_id_present=%t target_found=%t", page.Page.Number, page.Page.Size, page.Page.Total, len(page.Items), page.TableauRequestID != "", found)
	}
	t.Logf("sanitized workbook evidence: exact_luid=%s exact_tags=%d filtered_page_size=%d filtered_total=%d filtered_returned=%d", hashedWorkbookValue(item.LUID), len(item.Tags), page.Page.Size, page.Page.Total, len(page.Items))
}

func liveWorkbookConnection(t *testing.T) (config.Environment, coreauth.Session, *tableau.Transport) {
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
		t.Skip("set TADX_LIVE_ENVIRONMENT to run the live workbook contract test")
	}
	environment, err := configuration.ResolveEnvironment(alias)
	if err != nil {
		t.Fatal(err)
	}
	expectedSite := strings.TrimSpace(os.Getenv("TADX_LIVE_SITE_CONTENT_URL"))
	if expectedSite == "" {
		t.Skip("set TADX_LIVE_SITE_CONTENT_URL to guard the live workbook contract test")
	}
	if environment.SiteContentURL != expectedSite {
		t.Fatalf("live workbook contract expected the configured site content URL")
	}
	if _, ok := os.LookupEnv(environment.Auth.PATNameEnv); !ok {
		t.Skip("configured PAT name variable is not available in this process")
	}
	if _, ok := os.LookupEnv(environment.Auth.PATSecretEnv); !ok {
		t.Skip("configured PAT secret variable is not available in this process")
	}
	transport := tableau.NewTransport(http.DefaultClient, environment.APIVersion, func() string { return "workbook-inventory-live-contract" })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(context.Background(), coreauth.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv})
	if err != nil {
		t.Fatal(err)
	}
	return environment, session, transport
}

func hashedWorkbookValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum[:8])
}
