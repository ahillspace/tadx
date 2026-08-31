package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
)

type testSession struct {
	token    string
	siteLUID string
}

func (s testSession) String() string            { return "authenticated Tableau session" }
func (s testSession) SiteLUID() string          { return s.siteLUID }
func (s testSession) UserLUID() string          { return "user-1" }
func (s testSession) Authorize(r *http.Request) { r.Header.Set(coreauth.TableauAuthHeader, s.token) }

type capturedGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func TestClientTraversesAllConnectionsAndReturnsDeterministicRESTIdentities(t *testing.T) {
	var mu sync.Mutex
	var requests []capturedGraphQLRequest
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"embedded-1"},"nodes":[{"id":"embedded-a","name":"A","parentPublishedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"parent-1"},"nodes":[{"luid":"ds-b","name":"Beta"}]}}]}}]}}}`,
		`{"data":{"embeddedDatasourcesConnection":{"totalCount":1,"nodes":[{"id":"embedded-a","name":"A","parentPublishedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":"parent-2"},"nodes":[{"luid":"ds-a","name":"Alpha"}]}}]}}}`,
		`{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":"embedded-2"},"nodes":[{"id":"embedded-b","name":"B","parentPublishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":"parent-3"},"nodes":[{"luid":"ds-b","name":"Beta"}]}}]}}]}}}`,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/metadata/graphql" {
			t.Errorf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		if request.Header.Get("Content-Type") != "application/json" || request.Header.Get("Accept") != "application/json" {
			t.Errorf("content headers = %q, %q", request.Header.Get("Content-Type"), request.Header.Get("Accept"))
		}
		if request.Header.Get(coreauth.TableauAuthHeader) != "session-token" {
			t.Errorf("authorization header was not applied")
		}
		var payload capturedGraphQLRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		index := len(requests)
		requests = append(requests, payload)
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tableau-Request-Id", fmt.Sprintf("request-%d", index+1))
		_, _ = io.WriteString(writer, responses[index])
	}))
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	got, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []PublishedDatasource{{LUID: "ds-a", Name: "Alpha", SiteLUID: "site-1"}, {LUID: "ds-b", Name: "Beta", SiteLUID: "site-1"}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("published datasources = %#v, want %#v", got, want)
	}
	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(requests))
	}
	if !strings.Contains(requests[0].Query, "workbooksConnection") || !strings.Contains(requests[0].Query, "parentPublishedDatasourcesConnection") {
		t.Fatalf("primary query did not traverse the captured contract: %s", requests[0].Query)
	}
	if requests[0].Variables["workbookLuid"] != "wb-1" || requests[0].Variables["embeddedAfter"] != nil || requests[0].Variables["pageSize"] != float64(defaultPageSize) {
		t.Fatalf("first variables = %#v", requests[0].Variables)
	}
	if !strings.Contains(requests[1].Query, "embeddedDatasourcesConnection") || requests[1].Variables["embeddedDatasourceId"] != "embedded-a" || requests[1].Variables["parentAfter"] != "parent-1" {
		t.Fatalf("parent pagination request = %#v", requests[1])
	}
	if requests[2].Variables["embeddedAfter"] != "embedded-1" {
		t.Fatalf("second embedded cursor = %#v", requests[2].Variables)
	}
}

func TestClientRejectsRepeatedParentNodeAcrossPages(t *testing.T) {
	responses := []string{
		`{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"parent-1"},"nodes":[{"luid":"ds-1","name":"Sales"}]}}]}}]}}}`,
		`{"data":{"embeddedDatasourcesConnection":{"totalCount":1,"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"luid":"ds-1","name":"Sales"}]}}]}}}`,
	}
	requestIndex := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tableau-Request-Id", fmt.Sprintf("overlap-request-%d", requestIndex+1))
		_, _ = io.WriteString(writer, responses[requestIndex])
		requestIndex++
	}))
	defer server.Close()

	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
	if err == nil || !strings.Contains(err.Error(), `duplicate parent published datasource REST LUID "ds-1"`) {
		t.Fatalf("error = %v", err)
	}
	if tableau.RequestID(err) != "overlap-request-2" {
		t.Fatalf("request ID = %q", tableau.RequestID(err))
	}
}

func TestClientRejectsGraphQLErrorsWarningsAndPartialData(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		retryable      bool
		wantCodes      []string
		wantMessages   []string
		wantCorrection string
	}{
		{
			name: "permanent errors preserve every issue", retryable: false,
			body:      `{"data":{"workbooksConnection":{"totalCount":0,"nodes":[]}},"errors":[{"message":"access denied","extensions":{"code":"ACCESS_DENIED"}},{"message":"query is invalid","extensions":{"code":"INVALID_ARGUMENT"}}]}`,
			wantCodes: []string{"ACCESS_DENIED", "INVALID_ARGUMENT"}, wantMessages: []string{"access denied", "query is invalid"}, wantCorrection: "access",
		},
		{
			name: "incomplete warnings in errors are retryable", retryable: true,
			body:      `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[]}},"errors":[{"message":"partial nodes","extensions":{"code":"NODE_LIMIT_EXCEEDED","severity":"WARNING"}},{"message":"indexing","extensions":{"code":"BACKFILL_RUNNING","severity":"WARNING"}}]}`,
			wantCodes: []string{"NODE_LIMIT_EXCEEDED", "BACKFILL_RUNNING"}, wantMessages: []string{"partial nodes", "indexing"}, wantCorrection: "retry",
		},
		{
			name: "top-level warnings preserve every issue", retryable: true,
			body:      `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[]}},"warnings":[{"message":"partial results","code":"TIME_LIMIT_EXCEEDED"},{"message":"throttled","extensions":{"code":"RATE_EXCEEDED"}}]}`,
			wantCodes: []string{"TIME_LIMIT_EXCEEDED", "RATE_EXCEEDED"}, wantMessages: []string{"partial results", "throttled"}, wantCorrection: "retry",
		},
		{
			name: "permission warning is permanent", retryable: false,
			body:      `{"warnings":[{"message":"sort omitted","code":"USER_VISIBILITY_IS_LIMITED"}]}`,
			wantCodes: []string{"USER_VISIBILITY_IS_LIMITED"}, wantMessages: []string{"sort omitted"}, wantCorrection: "permission",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := metadataServer(t, test.body, "graphql-request")
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
			if err == nil || tableau.RequestID(err) != "graphql-request" {
				t.Fatalf("error = %v, request ID = %q", err, tableau.RequestID(err))
			}
			var advice interface {
				Retryable() bool
				CorrectiveAction() string
			}
			if !errors.As(err, &advice) || advice.Retryable() != test.retryable || !strings.Contains(strings.ToLower(advice.CorrectiveAction()), test.wantCorrection) {
				t.Fatalf("retry advice = %#v, error = %v", advice, err)
			}
			for _, code := range test.wantCodes {
				if !strings.Contains(err.Error(), code) {
					t.Errorf("error omitted code %q: %v", code, err)
				}
			}
			for _, message := range test.wantMessages {
				if !strings.Contains(err.Error(), message) {
					t.Errorf("error omitted message %q: %v", message, err)
				}
			}
		})
	}
}

func TestClientClassifiesMalformedGraphQLAsPermanentProtocolFailure(t *testing.T) {
	server := metadataServer(t, `{`, "decode-request")
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
	var advice interface {
		Retryable() bool
		CorrectiveAction() string
	}
	if err == nil || !errors.As(err, &advice) || advice.Retryable() || !strings.Contains(strings.ToLower(advice.CorrectiveAction()), "inspect") {
		t.Fatalf("error = %v, retry advice = %#v", err, advice)
	}
	if tableau.RequestID(err) != "decode-request" {
		t.Fatalf("request ID = %q", tableau.RequestID(err))
	}
}

func TestClientRequiresOneExactWorkbookAndCompletePagination(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing", body: `{"data":{"workbooksConnection":{"totalCount":0,"nodes":[]}}}`},
		{name: "ambiguous", body: `{"data":{"workbooksConnection":{"totalCount":2,"nodes":[{"luid":"wb-1"},{"luid":"wb-1"}]}}}`},
		{name: "wrong LUID", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-other"}]}}}`},
		{name: "missing embedded cursor", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":true},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`},
		{name: "missing embedded page info", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":0,"nodes":[]}}]}}}`},
		{name: "missing embedded nodes", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false}}}]}}}`},
		{name: "missing embedded total", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}`},
		{name: "missing embedded has next page", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":0,"pageInfo":{},"nodes":[]}}]}}}`},
		{name: "inconsistent embedded total", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`},
		{name: "inconsistent parent total", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"luid":"ds-1","name":"Sales"}]}}]}}]}}}`},
		{name: "missing parent page info", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":0,"nodes":[]}}]}}]}}}`},
		{name: "missing parent nodes", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false}}}]}}]}}}`},
		{name: "missing parent total", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`},
		{name: "missing parent has next page", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":0,"pageInfo":{},"nodes":[]}}]}}]}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := metadataServer(t, test.body, "protocol-request")
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
			if err == nil || tableau.RequestID(err) != "protocol-request" {
				t.Fatalf("error = %v, request ID = %q", err, tableau.RequestID(err))
			}
		})
	}
}

func TestClientRejectsBlankRESTLUIDAndConflictingLabels(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "blank LUID", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"luid":" ","name":"Sales"}]}}]}}]}}}`},
		{name: "conflicting labels", body: `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-1","parentPublishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"luid":"ds-1","name":"Sales"}]}},{"id":"embedded-2","parentPublishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"luid":"ds-1","name":"Revenue"}]}}]}}]}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := metadataServer(t, test.body, "identity-request")
			defer server.Close()
			client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
			_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.Split(test.name, " ")[0]) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestClientBoundsResponsesAndRedactsSessionSecrets(t *testing.T) {
	const token = "session-secret"
	t.Run("bounded response", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("X-Tableau-Request-Id", "large-request")
			_, _ = writer.Write(make([]byte, maxResponseBytes+1))
		}))
		defer server.Close()
		client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: token, siteLUID: "site-1"}, server.URL)
		_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
		if err == nil || !strings.Contains(err.Error(), "exceeded") || tableau.RequestID(err) != "large-request" {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("redacted GraphQL diagnostic", func(t *testing.T) {
		server := metadataServer(t, `{"errors":[{"message":"rejected `+token+`","extensions":{"code":"ACCESS_DENIED"}}]}`, "request-"+token)
		defer server.Close()
		client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: token, siteLUID: "site-1"}, server.URL)
		_, err := client.DirectPublishedDatasources(context.Background(), "wb-1")
		if err == nil || strings.Contains(err.Error(), token) || strings.Contains(tableau.RequestID(err), token) {
			t.Fatalf("secret leaked through error: %v, request ID %q", err, tableau.RequestID(err))
		}
	})
}

func TestClientRejectsBlankWorkbookLUIDBeforeRequest(t *testing.T) {
	client := NewClient(tableau.NewTransport(nil, "3.29", nil), testSession{siteLUID: "site-1"}, "https://tableau.example")
	_, err := client.DirectPublishedDatasources(context.Background(), " ")
	if err == nil || !strings.Contains(err.Error(), "workbook LUID") {
		t.Fatalf("error = %v", err)
	}
}

func metadataServer(t *testing.T, body, requestID string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.EscapedPath() != "/api/metadata/graphql" {
			t.Errorf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tableau-Request-Id", requestID)
		_, _ = io.WriteString(writer, body)
	}))
}
