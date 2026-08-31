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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); err == nil || err.Error() != "authenticated datasource client is not configured" {
				t.Fatalf("error = %v", err)
			}
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
