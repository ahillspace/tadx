package datasource_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type session struct{}

func (session) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", "token") }
func (session) SiteLUID() string                { return "site-1" }
func (session) UserLUID() string                { return "user-1" }
func (session) String() string                  { return "session" }

func TestClientUpdatesOnlyExplicitDatasourceFields(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.EscapedPath() != "/api/3.29/sites/site-1/datasources/ds-1" || request.URL.RawQuery != "" {
			t.Fatalf("request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		want := `<tsRequest><datasource name="Renamed"><project id="project-2"></project></datasource></tsRequest>`
		if string(body) != want || strings.Contains(string(body), "owner") {
			t.Fatalf("body = %q, want %q", body, want)
		}
		writer.Header().Set("X-Tableau-Request-Id", "request-update")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-1" name="Renamed"><project id="project-2" name="New"/><owner id="owner-1"/></datasource></tsResponse>`)
	}))
	defer server.Close()
	name, project := "Renamed", "project-2"
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Update(context.Background(), tableaudatasource.UpdateRequest{LUID: "ds-1", Name: &name, ProjectLUID: &project})
	if err != nil || result.Status != "succeeded" || result.DatasourceName != name || result.ProjectLUID != project || result.TableauRequestID != "request-update" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestClientRejectsIncompleteDatasourceUpdatesBeforeSending(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	empty := ""
	for _, input := range []tableaudatasource.UpdateRequest{{LUID: "ds-1"}, {Name: &empty}, {LUID: "ds-1", ProjectLUID: &empty}} {
		if _, err := client.Update(context.Background(), input); err == nil {
			t.Fatalf("Update(%#v) succeeded", input)
		}
	}
	if requests != 0 {
		t.Fatalf("requests = %d", requests)
	}
}

func TestClientRejectsUnconfiguredDatasourceReads(t *testing.T) {
	client := tableaudatasource.NewClient(nil, nil, "")
	tests := []struct {
		name string
		call func() error
	}{
		{name: "get", call: func() error {
			_, err := client.Get(context.Background(), "ds-1")
			return err
		}},
		{name: "download", call: func() error {
			_, err := client.Download(context.Background(), "ds-1", nil)
			return err
		}},
		{name: "list", call: func() error {
			_, err := client.List(context.Background(), tableaudatasource.ListRequest{PageNumber: 1, PageSize: 100})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil || err.Error() != "authenticated datasource client is not configured" {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestClientListsOneBoundedDatasourcePage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/3.29/sites/site-1/datasources" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.URL.Query(); got.Get("pageNumber") != "2" || got.Get("pageSize") != "2" || got.Get("filter") != "name:eq:Sales" {
			t.Fatalf("query = %v", got)
		}
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("X-Tableau-Request-Id", "datasource-list-request")
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="2" pageSize="2" totalAvailable="3"/><datasources><datasource id="ds-3" name="Sales"><project id="project-2" name="Shared"/></datasource></datasources></tsResponse>`)
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.List(context.Background(), tableaudatasource.ListRequest{PageNumber: 2, PageSize: 2, Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 2 || page.Size != 2 || page.Total != 3 || len(page.Items) != 1 {
		t.Fatalf("page = %#v", page)
	}
	item := page.Items[0]
	if item.LUID != "ds-3" || item.Name != "Sales" || item.ProjectLUID != "project-2" || item.ProjectName != "Shared" {
		t.Fatalf("item = %#v", item)
	}
}

func TestClientListsRichDatasourceMetadataWithStableFiltersAndRequestID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("filter") != "name:eq:Sales,ownerName:eq:owner,projectName:eq:Ops,type:eq:hyper,tags:eq:daily,updatedAt:gte:2026-01-01T00:00:00Z,updatedAt:lte:2026-09-01T00:00:00Z" {
			t.Fatalf("filter = %q", query.Get("filter"))
		}
		if query.Get("sort") != "name:asc,updatedAt:asc" {
			t.Fatalf("sort = %q", query.Get("sort"))
		}
		writer.Header().Set("X-Tableau-Request-Id", "datasource-list-rich-request")
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="1"/><datasources><datasource id="ds-1" name="Sales" description="Sales data" type="hyper" contentUrl="sales" size="42" createdAt="2026-08-01T00:00:00Z" updatedAt="2026-09-01T00:00:00Z" encryptExtracts="false" hasExtracts="true" isCertified="true" certificationNote="Reviewed" useRemoteQueryAgent="false" webpageUrl="https://tableau.example/datasources/1"><project id="project-1" name="Ops"/><owner id="user-1"/><tags><tag label="daily"/></tags><askData enablement="Enabled"/></datasource></datasources></tsResponse>`)
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.List(context.Background(), tableaudatasource.ListRequest{
		PageNumber: 1, PageSize: 1, Name: "Sales", OwnerName: "owner", ProjectLUID: "project-1", ProjectName: "Ops", Type: "hyper", Tag: "daily",
		UpdatedAfter: "2026-01-01T00:00:00Z", UpdatedBefore: "2026-09-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	item := page.Items[0]
	if page.TableauRequestID != "datasource-list-rich-request" || item.Description != "Sales data" || item.OwnerLUID != "user-1" || item.ContentURL != "sales" || item.Type != "hyper" || item.Size == nil || *item.Size != 42 || item.HasExtracts == nil || !*item.HasExtracts || len(item.Tags) != 1 || item.Tags[0] != "daily" {
		t.Fatalf("page = %#v", page)
	}
}

func TestClientResolvesDatasourceContentURLsToClassicLUIDsInOneBatch(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		query := request.URL.Query()
		if query.Get("filter") != "contentUrl:in:[alpha,beta]" || query.Get("pageNumber") != "1" || query.Get("pageSize") != "2" {
			t.Fatalf("query = %v", query)
		}
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="2"/><datasources><datasource id="ds-b" name="Beta" contentUrl="beta"><project id="project-1" name="Ops"/></datasource><datasource id="ds-a" name="Alpha" contentUrl="alpha"><project id="project-1" name="Ops"/></datasource></datasources></tsResponse>`)
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	resolved, err := client.ResolveContentURLs(context.Background(), []string{"beta", "alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 || resolved["alpha"] != "ds-a" || resolved["beta"] != "ds-b" {
		t.Fatalf("requests=%d resolved=%v", requests, resolved)
	}
}

func TestClientFailsClosedWhenContentURLResolutionIsIncomplete(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="1"/><datasources><datasource id="ds-a" name="Alpha" contentUrl="alpha"><project id="project-1" name="Ops"/></datasource></datasources></tsResponse>`)
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if _, err := client.ResolveContentURLs(context.Background(), []string{"alpha", "missing"}); err == nil || !strings.Contains(err.Error(), "did not resolve") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientAvoidsLiveRejectedDatasourceContentURLSort(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sortValue := request.URL.Query().Get("sort")
		if strings.Contains(sortValue, "contentUrl") {
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `<tsResponse><error code="400066"><summary>Bad Request</summary><detail>The sort contains a key 'contentUrl' which is not recognized for this api version.</detail></error></tsResponse>`)
			return
		}
		if sortValue != "name:asc,updatedAt:asc" {
			t.Fatalf("sort = %q", sortValue)
		}
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="0"/><datasources/></tsResponse>`)
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if _, err := client.List(context.Background(), tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestClientRejectsInvalidDatasourceListInput(t *testing.T) {
	client := tableaudatasource.NewClient(tableau.NewTransport(http.DefaultClient, "3.29", nil), session{}, "https://example.invalid")
	tests := []struct {
		name  string
		input tableaudatasource.ListRequest
	}{
		{name: "zero page", input: tableaudatasource.ListRequest{PageSize: 100}},
		{name: "zero size", input: tableaudatasource.ListRequest{PageNumber: 1}},
		{name: "oversize", input: tableaudatasource.ListRequest{PageNumber: 1, PageSize: 1001}},
		{name: "unsafe comma", input: tableaudatasource.ListRequest{PageNumber: 1, PageSize: 100, Name: "Sales,Other"}},
		{name: "unsafe ampersand", input: tableaudatasource.ListRequest{PageNumber: 1, PageSize: 100, Name: "Sales&Other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := client.List(context.Background(), test.input); err == nil {
				t.Fatal("List() succeeded")
			}
		})
	}
}

func TestClientRejectsInvalidDatasourceListResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed XML", body: `<tsResponse><pagination`, want: "decode datasource list response"},
		{name: "missing pagination", body: `<tsResponse><datasources/></tsResponse>`, want: "pagination elements"},
		{name: "duplicate pagination", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="0"/><pagination pageNumber="1" pageSize="2" totalAvailable="0"/><datasources/></tsResponse>`, want: "pagination elements"},
		{name: "missing container", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="0"/></tsResponse>`, want: "datasources elements"},
		{name: "wrong page", body: `<tsResponse><pagination pageNumber="2" pageSize="2" totalAvailable="1"/><datasources/></tsResponse>`, want: "page number"},
		{name: "incomplete identity", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="1"/><datasources><datasource name="Sales"><project id="p-1" name="Shared"/></datasource></datasources></tsResponse>`, want: "incomplete authoritative identity"},
		{name: "missing project name", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="1"/><datasources><datasource id="ds-1" name="Sales"><project id="p-1"/></datasource></datasources></tsResponse>`, want: "incomplete authoritative identity"},
		{name: "conflicting duplicate", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="2"/><datasources><datasource id="ds-1" name="Sales"><project id="p-1" name="One"/></datasource><datasource id="ds-1" name="Changed"><project id="p-1" name="One"/></datasource></datasources></tsResponse>`, want: `conflicting records for LUID "ds-1"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("X-Tableau-Request-Id", "datasource-list-protocol-request")
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()

			client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			_, err := client.List(context.Background(), tableaudatasource.ListRequest{PageNumber: 1, PageSize: 2})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			assertProtocolContext(t, err, http.StatusOK, "datasource-list-protocol-request")
		})
	}
}

func TestClientGetsDatasourceByAuthoritativeLUID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/3.29/sites/site-1/datasources/ds-1" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Accept") != "application/xml" || request.Header.Get("X-Tableau-Auth") != "token" {
			t.Fatalf("headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("X-Tableau-Request-Id", "datasource-get-request")
		_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-1" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	datasource, err := client.Get(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if datasource.LUID != "ds-1" || datasource.Name != "Sales" || datasource.ProjectLUID != "project-1" || datasource.ProjectName != "Analytics" {
		t.Fatalf("datasource = %#v", datasource)
	}
}

func TestClientGetsRichDatasourceMetadataAndRequestID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "datasource-get-rich-request")
		_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-1" name="Sales" description="Sales data" type="hyper" contentUrl="sales" updatedAt="2026-09-01T00:00:00Z"><project id="project-1" name="Ops"/><owner id="user-1"/><tags><tag label="daily"/></tags><askData enablement="Enabled"/></datasource></tsResponse>`)
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	item, err := client.Get(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if item.TableauRequestID != "datasource-get-rich-request" || item.Description != "Sales data" || item.OwnerLUID != "user-1" || item.AskDataEnablement != "Enabled" || len(item.Tags) != 1 {
		t.Fatalf("datasource = %#v", item)
	}
}

func TestClientRejectsInvalidDatasourceIdentityResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed XML", body: `<tsResponse><datasource`, want: "decode datasource response"},
		{name: "missing LUID", body: `<tsResponse><datasource name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`, want: `returned LUID "", expected "ds-1"`},
		{name: "different LUID", body: `<tsResponse><datasource id="ds-2" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`, want: `returned LUID "ds-2", expected "ds-1"`},
		{name: "missing name", body: `<tsResponse><datasource id="ds-1"><project id="project-1" name="Analytics"/></datasource></tsResponse>`, want: "omitted name"},
		{name: "missing project LUID", body: `<tsResponse><datasource id="ds-1" name="Sales"><project name="Analytics"/></datasource></tsResponse>`, want: "omitted project LUID"},
		{name: "missing project name", body: `<tsResponse><datasource id="ds-1" name="Sales"><project id="project-1"/></datasource></tsResponse>`, want: "omitted project name"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("X-Tableau-Request-Id", "datasource-protocol-request")
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()

			client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			_, err := client.Get(context.Background(), "ds-1")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			assertProtocolContext(t, err, http.StatusOK, "datasource-protocol-request")
		})
	}
}

func TestClientDownloadsNativeDatasourceAndFilename(t *testing.T) {
	include := func(value bool) *bool { return &value }
	tests := []struct {
		name           string
		includeExtract *bool
		wantQuery      string
		filename       string
	}{
		{name: "tdsx with default extract behavior", filename: "Sales.tdsx"},
		{name: "tds with extracts included", includeExtract: include(true), wantQuery: "includeExtract=true", filename: "Sales.tds"},
		{name: "tdsx without extracts", includeExtract: include(false), wantQuery: "includeExtract=false", filename: "Sales.tdsx"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet || request.URL.Path != "/api/3.29/sites/site-1/datasources/ds-1/content" || request.URL.RawQuery != test.wantQuery {
					t.Fatalf("request URI = %s %s", request.Method, request.URL.RequestURI())
				}
				writer.Header().Set("Content-Disposition", `name="tableau_datasource"; filename="`+test.filename+`"`)
				writer.Header().Set("Content-Type", "application/octet-stream")
				writer.Header().Set("X-Tableau-Request-Id", "datasource-download-request")
				_, _ = writer.Write([]byte("native-datasource"))
			}))
			defer server.Close()

			client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			download, err := client.Download(context.Background(), "ds-1", test.includeExtract)
			if err != nil {
				t.Fatal(err)
			}
			if download.Filename != test.filename || download.ContentType != "application/octet-stream" || !bytes.Equal(download.Content, []byte("native-datasource")) || download.TableauRequestID != "datasource-download-request" {
				t.Fatalf("download = %#v", download)
			}
		})
	}
}

func TestClientRejectsInvalidDatasourceDownloadFilename(t *testing.T) {
	tests := []struct {
		name        string
		disposition string
		want        string
	}{
		{name: "missing", want: "omitted filename"},
		{name: "unsupported extension", disposition: `name="tableau_datasource"; filename="Sales.csv"`, want: `unsupported filename "Sales.csv"`},
		{name: "directory path", disposition: `name="tableau_datasource"; filename="../Sales.tdsx"`, want: `invalid filename "../Sales.tdsx"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Disposition", test.disposition)
				writer.Header().Set("X-Tableau-Request-Id", "datasource-filename-request")
				_, _ = writer.Write([]byte("native-datasource"))
			}))
			defer server.Close()

			client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			_, err := client.Download(context.Background(), "ds-1", nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			assertProtocolContext(t, err, http.StatusOK, "datasource-filename-request")
		})
	}
}

func TestClientBoundsDatasourceDownloadAndPreservesRequestID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Disposition", `name="tableau_datasource"; filename="Sales.tdsx"`)
		writer.Header().Set("X-Tableau-Request-Id", "datasource-large-request")
		_, _ = writer.Write([]byte("too-large"))
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetMaxDownloadBytes(4)
	_, err := client.Download(context.Background(), "ds-1", nil)
	if err == nil || !strings.Contains(err.Error(), "exceeded 4-byte limit") {
		t.Fatalf("error = %v", err)
	}
	var status interface{ HTTPStatus() int }
	if !errors.As(err, &status) || status.HTTPStatus() != http.StatusOK || tableau.RequestID(err) != "datasource-large-request" {
		t.Fatalf("bounded download error = %#v", err)
	}
}

func TestClientPreservesDatasourceUpstreamError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "datasource-upstream-request")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, `<tsResponse><error code="404004"><summary>Data source not found</summary><detail>The data source does not exist.</detail></error></tsResponse>`)
	}))
	defer server.Close()

	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	_, err := client.Get(context.Background(), "ds-1")
	var upstream *tableau.UpstreamError
	if err == nil || !errors.As(err, &upstream) || upstream.StatusCode != http.StatusNotFound || upstream.Code != "404004" || tableau.RequestID(err) != "datasource-upstream-request" {
		t.Fatalf("upstream error = %#v", err)
	}
}

func assertProtocolContext(t *testing.T, err error, statusCode int, requestID string) {
	t.Helper()
	var protocol *tableau.ProtocolError
	if err == nil || !errors.As(err, &protocol) || protocol.HTTPStatus() != statusCode || tableau.RequestID(err) != requestID {
		t.Fatalf("protocol error = %#v", err)
	}
}
