package metadata

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

var workbookLineageRequest = CaptureRequest{Kind: KindWorkbook, RESTLUID: "wb-1", Direction: DirectionUpstream}

func TestClientPreservesEveryFatalGraphQLIssue(t *testing.T) {
	server := metadataServer(t, `{"data":{"workbooksConnection":{"totalCount":0,"nodes":[]}},"errors":[{"message":"access denied","extensions":{"code":"ACCESS_DENIED"}},{"message":"query is invalid","extensions":{"code":"INVALID_ARGUMENT"}}]}`, "graphql-request")
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	_, err := client.CaptureLineage(context.Background(), workbookLineageRequest)
	if err == nil || tableau.RequestID(err) != "graphql-request" {
		t.Fatalf("error = %v, request ID = %q", err, tableau.RequestID(err))
	}
	var advice interface {
		Retryable() bool
		CorrectiveAction() string
	}
	if !errors.As(err, &advice) || advice.Retryable() || !strings.Contains(strings.ToLower(advice.CorrectiveAction()), "access") {
		t.Fatalf("retry advice = %#v, error = %v", advice, err)
	}
	for _, want := range []string{"ACCESS_DENIED", "INVALID_ARGUMENT", "access denied", "query is invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error omitted %q: %v", want, err)
		}
	}
}

func TestClientClassifiesMalformedGraphQLAsPermanentProtocolFailure(t *testing.T) {
	server := metadataServer(t, `{`, "decode-request")
	defer server.Close()
	client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: "session-token", siteLUID: "site-1"}, server.URL)
	_, err := client.CaptureLineage(context.Background(), workbookLineageRequest)
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

func TestClientBoundsResponsesAndRedactsSessionSecrets(t *testing.T) {
	const token = "session-secret"
	t.Run("bounded response", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("X-Tableau-Request-Id", "large-request")
			_, _ = writer.Write(make([]byte, maxResponseBytes+1))
		}))
		defer server.Close()
		client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: token, siteLUID: "site-1"}, server.URL)
		_, err := client.CaptureLineage(context.Background(), workbookLineageRequest)
		if err == nil || !strings.Contains(err.Error(), "exceeded") || tableau.RequestID(err) != "large-request" {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("redacted GraphQL diagnostic", func(t *testing.T) {
		server := metadataServer(t, `{"errors":[{"message":"rejected `+token+`","extensions":{"code":"ACCESS_DENIED"}}]}`, "request-"+token)
		defer server.Close()
		client := NewClient(tableau.NewTransport(server.Client(), "3.29", nil), testSession{token: token, siteLUID: "site-1"}, server.URL)
		_, err := client.CaptureLineage(context.Background(), workbookLineageRequest)
		if err == nil || strings.Contains(err.Error(), token) || strings.Contains(tableau.RequestID(err), token) {
			t.Fatalf("secret leaked through error: %v, request ID %q", err, tableau.RequestID(err))
		}
	})
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
