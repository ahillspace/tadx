//go:build live

package datasource_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

const liveDatasourceDownloadLimit = 256 * 1024 * 1024

// TestLiveDatasourceListContract exercises the published datasource list, pagination,
// and name-filter surface introduced for lineage.pull --kind published_datasource.
func TestLiveDatasourceListContract(t *testing.T) {
	environment, session, transport := liveDatasourceConnection(t)
	client := tableaudatasource.NewClient(transport, session, environment.URL)
	ctx := context.Background()

	page, err := client.List(ctx, tableaudatasource.ListRequest{PageNumber: 1, PageSize: 25})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 1 || page.Size <= 0 || page.Total < 0 {
		t.Fatalf("invalid live datasource pagination: %#v", page)
	}
	if page.Total < len(page.Items) {
		t.Fatalf("live datasource total %d is smaller than returned item count %d", page.Total, len(page.Items))
	}
	if len(page.Items) == 0 {
		t.Skip("configured site exposes no published datasources to exercise the list contract")
	}

	first := page.Items[0]
	for _, item := range page.Items {
		if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" ||
			strings.TrimSpace(item.ProjectLUID) == "" || strings.TrimSpace(item.ProjectName) == "" {
			t.Fatal("live datasource response omitted authoritative identity")
		}
	}

	// Pagination: when more than one page exists, page 2 must return the residual items.
	if page.Total > page.Size {
		next, nextErr := client.List(ctx, tableaudatasource.ListRequest{PageNumber: 2, PageSize: page.Size})
		if nextErr != nil {
			t.Fatal(nextErr)
		}
		if next.Number != 2 || next.Size != page.Size || next.Total != page.Total {
			t.Fatalf("live datasource second page pagination drifted: %#v", next)
		}
		if len(next.Items) == 0 {
			t.Fatal("live datasource second page returned no items despite a total exceeding one page")
		}
		if next.Items[0].LUID == first.LUID {
			t.Fatal("live datasource second page repeated the first page's leading item")
		}
	}

	// Name filter: filtering on an observed name must return that exact datasource.
	filtered, err := client.List(ctx, tableaudatasource.ListRequest{PageNumber: 1, PageSize: 25, Name: first.Name})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Number != 1 || filtered.Size <= 0 {
		t.Fatalf("invalid live datasource filtered pagination: %#v", filtered)
	}
	found := false
	for _, item := range filtered.Items {
		if item.Name != first.Name {
			t.Fatalf("live datasource name filter %q returned unrelated datasource %q", first.Name, item.Name)
		}
		if item.LUID == first.LUID {
			found = true
		}
	}
	if !found {
		t.Fatalf("live datasource name filter %q did not return the expected datasource", hashLiveDatasourceID(first.LUID))
	}

	t.Logf(
		"sanitized datasource list evidence: method=GET path=/api/{version}/sites/{site}/datasources page=1 size=%d total=%d returned=%d filtered_returned=%d first_luid=%s first_project=%s",
		page.Size, page.Total, len(page.Items), len(filtered.Items),
		hashLiveDatasourceID(first.LUID), hashLiveDatasourceID(first.ProjectLUID),
	)
}

// TestLiveDatasourceDownloadContract exercises the includeExtract native download path.
// It is gated on TADX_LIVE_DATASOURCE_LUID so it only runs against a known-good datasource.
func TestLiveDatasourceDownloadContract(t *testing.T) {
	datasourceLUID := strings.TrimSpace(os.Getenv("TADX_LIVE_DATASOURCE_LUID"))
	if datasourceLUID == "" {
		t.Skip("set TADX_LIVE_DATASOURCE_LUID to run the live datasource download contract test")
	}

	environment, session, transport := liveDatasourceConnection(t)
	client := tableaudatasource.NewClient(transport, session, environment.URL)
	client.SetMaxDownloadBytes(liveDatasourceDownloadLimit)
	ctx := context.Background()

	item, err := client.Get(ctx, datasourceLUID)
	if err != nil {
		t.Fatal(err)
	}
	if item.LUID != datasourceLUID {
		t.Fatalf("REST datasource LUID = %q, requested %q", item.LUID, datasourceLUID)
	}
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ProjectLUID) == "" || strings.TrimSpace(item.ProjectName) == "" {
		t.Fatal("live datasource get omitted authoritative identity")
	}

	// Exercise both includeExtract states of the download path around client.go:160.
	withExtract := true
	withoutExtract := false
	for _, include := range []*bool{nil, &withoutExtract, &withExtract} {
		download, downloadErr := client.Download(ctx, datasourceLUID, include)
		if downloadErr != nil {
			t.Fatalf("download includeExtract=%s failed: %v", describeIncludeExtract(include), downloadErr)
		}
		extension := strings.ToLower(filepath.Ext(download.Filename))
		if extension != ".tds" && extension != ".tdsx" {
			t.Fatalf("datasource download returned unsupported extension %q", extension)
		}
		if len(download.Content) == 0 {
			t.Fatalf("datasource download includeExtract=%s returned no content", describeIncludeExtract(include))
		}
		t.Logf(
			"sanitized datasource download evidence: luid=%s includeExtract=%s extension=%s content_type=%s bytes=%d content_sha256=%s request_id=%s",
			hashLiveDatasourceID(item.LUID), describeIncludeExtract(include), extension, download.ContentType,
			len(download.Content), fingerprintLiveDatasource(download.Content), hashLiveDatasourceID(download.TableauRequestID),
		)
	}
}

func describeIncludeExtract(include *bool) string {
	if include == nil {
		return "unset"
	}
	return fmt.Sprintf("%t", *include)
}

func liveDatasourceConnection(t *testing.T) (config.Environment, coreauth.Session, *tableau.Transport) {
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
	transport := tableau.NewTransport(http.DefaultClient, environment.APIVersion, func() string { return "datasource-live-contract" })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(context.Background(), coreauth.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv})
	if err != nil {
		t.Fatal(err)
	}
	return environment, session, transport
}

func hashLiveDatasourceID(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("sha256:%x", sum[:8])
}

func fingerprintLiveDatasource(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%x", sum[:8])
}
