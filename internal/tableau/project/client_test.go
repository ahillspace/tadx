package project_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

type session struct{}

func (session) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", "token") }
func (session) SiteLUID() string                { return "site/one" }
func (session) UserLUID() string                { return "user-1" }
func (session) String() string                  { return "session" }

type tokenSession string

func (value tokenSession) Authorize(request *http.Request) {
	request.Header.Set("X-Tableau-Auth", string(value))
}
func (tokenSession) SiteLUID() string { return "site/one" }
func (tokenSession) UserLUID() string { return "user-1" }
func (tokenSession) String() string   { return "session" }

func TestClientListsOneFilteredClassicPage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/3.29/sites/site%2Fone/projects" {
			t.Fatalf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		query := request.URL.Query()
		if query.Get("pageNumber") != "2" || query.Get("pageSize") != "2" {
			t.Fatalf("pagination query = %s", request.URL.RawQuery)
		}
		if got := query.Get("filter"); got != "name:eq:Operations,parentProjectId:eq:parent-1,ownerName:eq:Owner Name,topLevelProject:eq:false" {
			t.Fatalf("filter = %q", got)
		}
		if request.Header.Get("Accept") != "application/xml" || request.Header.Get("X-Tableau-Auth") != "token" || request.Header.Get("X-TADX-Correlation-ID") != "correlation-1" {
			t.Fatalf("headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="2" pageSize="2" totalAvailable="3"/><projects><project id="project-3" name="Operations" description="Nested project" parentProjectId="parent-1" topLevelProject="false" contentPermissions="LockedToProject" controllingPermissionsProjectId="control-1" createdAt="2026-01-02T03:04:05Z" updatedAt="2026-02-03T04:05:06Z"><owner id="owner-1"/><contentCounts projectCount="1" workbookCount="2" viewCount="3" datasourceCount="4"/></project></projects></tsResponse>`)
	}))
	defer server.Close()

	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", func() string { return "correlation-1" }), session{}, server.URL)
	topLevel := false
	page, err := client.List(context.Background(), tableauproject.ListRequest{
		PageNumber: 2,
		PageSize:   2,
		Name:       "Operations",
		ParentLUID: "parent-1",
		OwnerName:  "Owner Name",
		TopLevel:   &topLevel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Number != 2 || page.Size != 2 || page.Total != 3 || len(page.Items) != 1 {
		t.Fatalf("page = %#v", page)
	}
	project := page.Items[0]
	if project.LUID != "project-3" || project.Name != "Operations" || project.ParentLUID != "parent-1" || project.OwnerLUID != "owner-1" {
		t.Fatalf("project identity = %#v", project)
	}
	if project.TopLevel == nil || *project.TopLevel || project.ProjectCount == nil || *project.ProjectCount != 1 || project.WorkbookCount == nil || *project.WorkbookCount != 2 || project.ViewCount == nil || *project.ViewCount != 3 || project.DatasourceCount == nil || *project.DatasourceCount != 4 {
		t.Fatalf("project details = %#v", project)
	}
	if project.ContentPermissions != "LockedToProject" || project.ControllingPermissionsProjectID != "control-1" || project.CreatedAt == "" || project.UpdatedAt == "" {
		t.Fatalf("project metadata = %#v", project)
	}
}

func TestClientRejectsInvalidListRequestsBeforeSending(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)

	tests := []struct {
		name  string
		input tableauproject.ListRequest
	}{
		{name: "zero page number", input: tableauproject.ListRequest{PageSize: 100}},
		{name: "zero page size", input: tableauproject.ListRequest{PageNumber: 1}},
		{name: "page size above upstream maximum", input: tableauproject.ListRequest{PageNumber: 1, PageSize: 1001}},
		{name: "comma in filter", input: tableauproject.ListRequest{PageNumber: 1, PageSize: 100, Name: "Finance,Ops"}},
		{name: "ampersand in filter", input: tableauproject.ListRequest{PageNumber: 1, PageSize: 100, OwnerName: "A&B"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := client.List(context.Background(), test.input); err == nil {
				t.Fatal("List() succeeded")
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestClientRejectsMalformedProjectEnvelopes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "wrong root", body: `<response><pagination pageNumber="1" pageSize="2" totalAvailable="0"/><projects/></response>`},
		{name: "missing pagination", body: `<tsResponse><projects/></tsResponse>`},
		{name: "missing pagination attribute", body: `<tsResponse><pagination pageNumber="1" pageSize="2"/><projects/></tsResponse>`},
		{name: "malformed XML", body: `<tsResponse><pagination pageNumber="1"`},
		{name: "repeated pagination", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="0"/><pagination pageNumber="1" pageSize="2" totalAvailable="0"/><projects/></tsResponse>`},
		{name: "missing projects", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="0"/></tsResponse>`},
		{name: "repeated projects", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="0"/><projects/><projects/></tsResponse>`},
		{name: "missing project LUID", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="1"/><projects><project name="Ops"/></projects></tsResponse>`},
		{name: "missing project name", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="1"/><projects><project id="project-1"/></projects></tsResponse>`},
		{name: "wrong page number", body: `<tsResponse><pagination pageNumber="2" pageSize="2" totalAvailable="0"/><projects/></tsResponse>`},
		{name: "oversized returned page", body: `<tsResponse><pagination pageNumber="1" pageSize="3" totalAvailable="0"/><projects/></tsResponse>`},
		{name: "underfilled non-final page", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="3"/><projects><project id="project-1" name="One"/></projects></tsResponse>`},
		{name: "conflicting duplicate LUID", body: `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="2"/><projects><project id="project-1" name="One"/><project id="project-1" name="Two"/></projects></tsResponse>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("X-Tableau-Request-Id", "project-request")
				writer.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			_, err := client.List(context.Background(), tableauproject.ListRequest{PageNumber: 1, PageSize: 2})
			assertProtocolContext(t, err, "project-request")
		})
	}
}

func TestClientAcceptsAgreeingDuplicateRowsInServerOrder(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="3" totalAvailable="3"/><projects><project id="project-2" name="Two"/><project id="project-1" name="One"/><project id="project-1" name="One"/></projects></tsResponse>`)
	}))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.List(context.Background(), tableauproject.ListRequest{PageNumber: 1, PageSize: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 || page.Items[0].LUID != "project-2" || page.Items[1].LUID != "project-1" || page.Items[2].LUID != "project-1" {
		t.Fatalf("items = %#v", page.Items)
	}
}

func TestClientNormalizesAuthoritativeIdentityWhitespaceBeforeDuplicateChecks(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="2"/><projects><project id=" project-1 " name=" One "/><project id="project-1" name="One"/></projects></tsResponse>`)
	}))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	page, err := client.List(context.Background(), tableauproject.ListRequest{PageNumber: 1, PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].LUID != "project-1" || page.Items[0].Name != "One" {
		t.Fatalf("items = %#v", page.Items)
	}
}

func TestClientPreservesStructuredUpstreamError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "upstream-request")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `<tsResponse><error code="503000"><summary>Unavailable</summary><detail>Retry later</detail></error></tsResponse>`)
	}))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	_, err := client.List(context.Background(), tableauproject.ListRequest{PageNumber: 1, PageSize: 100})
	var upstream *tableau.UpstreamError
	if !errors.As(err, &upstream) || upstream.StatusCode != http.StatusServiceUnavailable || upstream.Code != "503000" || upstream.TableauRequestID != "upstream-request" || !upstream.Retryable() {
		t.Fatalf("upstream error = %#v", err)
	}
}

func TestClientRedactsSessionTokenFromProtocolErrors(t *testing.T) {
	secret := "sensitive-session-token"
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="2" totalAvailable="2"/><projects><project id="`+secret+`" name="One"/><project id="`+secret+`" name="Two"/></projects></tsResponse>`)
	}))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), tokenSession(secret), server.URL)
	_, err := client.List(context.Background(), tableauproject.ListRequest{PageNumber: 1, PageSize: 2})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error = %v", err)
	}
}

func assertProtocolContext(t *testing.T, err error, requestID string) {
	t.Helper()
	var protocol *tableau.ProtocolError
	if !errors.As(err, &protocol) || protocol.HTTPStatus() != http.StatusOK || tableau.RequestID(err) != requestID || !protocol.Retryable() {
		t.Fatalf("protocol error = %#v", err)
	}
}
