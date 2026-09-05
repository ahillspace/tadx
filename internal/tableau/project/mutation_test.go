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

func TestClientCreatesAndUpdatesProjectsWithExactContracts(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestNumber := requests.Add(1)
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if request.Header.Get("Content-Type") != "application/xml" || request.Header.Get("Accept") != "application/xml" {
			t.Fatalf("headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("X-Tableau-Request-Id", "project-mutation-request")
		switch requestNumber {
		case 1:
			if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/3.29/sites/site%2Fone/projects" || request.URL.RawQuery != "" {
				t.Fatalf("create request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			want := `<tsRequest><project name="Operations" description="Direct operations" parentProjectId="parent-1" contentPermissions="LockedToProject"></project></tsRequest>`
			if string(body) != want {
				t.Fatalf("create body = %q, want %q", body, want)
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><project id="project-1" name="Operations" description="Direct operations" parentProjectId="parent-1" contentPermissions="LockedToProject" controllingPermissionsProjectId="parent-1"><owner id="owner-1"/></project></tsResponse>`)
		case 2:
			if request.Method != http.MethodPut || request.URL.EscapedPath() != "/api/3.29/sites/site%2Fone/projects/project-1" || request.URL.RawQuery != "" {
				t.Fatalf("update request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
			}
			want := `<tsRequest><project name="Renamed" description="" contentPermissions="ManagedByOwner"></project></tsRequest>`
			if string(body) != want || strings.Contains(string(body), "parentProjectId") || strings.Contains(string(body), "owner") {
				t.Fatalf("update body = %q, want %q", body, want)
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, `<tsResponse><project id="project-1" name="Renamed" description="" parentProjectId="parent-1" contentPermissions="ManagedByOwner"><owner id="owner-1"/></project></tsResponse>`)
		default:
			t.Fatalf("unexpected request %d", requestNumber)
		}
	}))
	defer server.Close()

	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	created, err := client.Create(context.Background(), tableauproject.CreateRequest{
		Name: "Operations", Description: "Direct operations", ParentLUID: "parent-1", ContentPermissions: "LockedToProject",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != "succeeded" || created.Project.LUID != "project-1" || created.Project.ParentLUID != "parent-1" || created.TableauRequestID != "project-mutation-request" {
		t.Fatalf("created = %#v", created)
	}

	name, description, permissions := "Renamed", "", "ManagedByOwner"
	updated, err := client.Update(context.Background(), tableauproject.UpdateRequest{
		LUID: "project-1", Name: &name, Description: &description, ContentPermissions: &permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "succeeded" || updated.Project.LUID != "project-1" || updated.Project.Name != "Renamed" || updated.TableauRequestID != "project-mutation-request" {
		t.Fatalf("updated = %#v", updated)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestClientRejectsInvalidProjectMutationsBeforeSending(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)

	if _, err := client.Create(context.Background(), tableauproject.CreateRequest{}); err == nil {
		t.Fatal("Create() accepted a missing name")
	}
	if _, err := client.Create(context.Background(), tableauproject.CreateRequest{Name: "Operations/Reports"}); err == nil {
		t.Fatal("Create() accepted an unaddressable name")
	}
	if _, err := client.Update(context.Background(), tableauproject.UpdateRequest{LUID: "project-1"}); err == nil {
		t.Fatal("Update() accepted no fields")
	}
	name := "Renamed"
	if _, err := client.Update(context.Background(), tableauproject.UpdateRequest{Name: &name}); err == nil {
		t.Fatal("Update() accepted a missing LUID")
	}
	unaddressable := "Operations/Reports"
	if _, err := client.Update(context.Background(), tableauproject.UpdateRequest{LUID: "project-1", Name: &unaddressable}); err == nil {
		t.Fatal("Update() accepted an unaddressable name")
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestClientMovesProjectToParentAndTopLevel(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		wantParent := "parent-2"
		if requests == 2 {
			wantParent = ""
		}
		want := `<tsRequest><project parentProjectId="` + wantParent + `"></project></tsRequest>`
		if request.Method != http.MethodPut || string(body) != want {
			t.Fatalf("request=%s body=%q want=%q", request.Method, body, want)
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `<tsResponse><project id="project-1" name="Child" parentProjectId="`+wantParent+`"><owner id="owner-1"/></project></tsResponse>`)
	}))
	defer server.Close()
	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	parent := "parent-2"
	if result, err := client.Update(context.Background(), tableauproject.UpdateRequest{LUID: "project-1", ParentLUID: &parent}); err != nil || result.Project.ParentLUID != parent {
		t.Fatalf("parent result=%#v err=%v", result, err)
	}
	topLevel := ""
	if result, err := client.Update(context.Background(), tableauproject.UpdateRequest{LUID: "project-1", ParentLUID: &topLevel}); err != nil || result.Project.ParentLUID != "" {
		t.Fatalf("top-level result=%#v err=%v", result, err)
	}
}

func TestClientDeletesProjectWithExactLUID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.EscapedPath() != "/api/3.29/sites/site%2Fone/projects/project-1" || request.URL.RawQuery != "" {
			t.Fatalf("delete request = %s %s?%s", request.Method, request.URL.EscapedPath(), request.URL.RawQuery)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) != 0 {
			t.Fatalf("delete body = %q", body)
		}
		writer.Header().Set("X-Tableau-Request-Id", "project-delete-request")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Delete(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.ProjectLUID != "project-1" || result.TableauRequestID != "project-delete-request" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientRejectsNonEmptyProjectDeleteSuccessContract(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			t.Fatalf("method = %s", request.Method)
		}
		writer.Header().Set("X-Tableau-Request-Id", "project-delete-invalid-response")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `<tsResponse/>`)
	}))
	defer server.Close()

	client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Delete(context.Background(), "project-1")
	var protocol *tableau.ProtocolError
	if !errors.As(err, &protocol) || result.Status != "unknown" || result.ProjectLUID != "project-1" || tableau.RequestID(err) != "project-delete-invalid-response" {
		t.Fatalf("result=%#v error=%#v", result, err)
	}
}

func TestClientRejectsMalformedProjectMutationResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "wrong status", statusCode: http.StatusOK, body: `<tsResponse><project id="project-1" name="Operations"/></tsResponse>`},
		{name: "missing project", statusCode: http.StatusCreated, body: `<tsResponse/>`},
		{name: "missing LUID", statusCode: http.StatusCreated, body: `<tsResponse><project name="Operations"/></tsResponse>`},
		{name: "wrong name", statusCode: http.StatusCreated, body: `<tsResponse><project id="project-1" name="Other"/></tsResponse>`},
		{name: "wrong parent", statusCode: http.StatusCreated, body: `<tsResponse><project id="project-1" name="Operations" parentProjectId="other"/></tsResponse>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("X-Tableau-Request-Id", "project-protocol-request")
				writer.WriteHeader(test.statusCode)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()
			client := tableauproject.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			_, err := client.Create(context.Background(), tableauproject.CreateRequest{Name: "Operations", ParentLUID: "parent-1"})
			var protocol *tableau.ProtocolError
			if !errors.As(err, &protocol) || protocol.HTTPStatus() != test.statusCode || tableau.RequestID(err) != "project-protocol-request" || protocol.Retryable() {
				t.Fatalf("protocol error = %#v", err)
			}
		})
	}
}
