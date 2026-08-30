package tableau

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testSession struct{ token string }

func (s testSession) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", s.token) }
func (s testSession) SiteLUID() string                { return "site-luid" }
func (s testSession) UserLUID() string                { return "user-luid" }
func (s testSession) String() string                  { return "redacted session" }

func TestTransportAddsStableHeadersAndCapturesTableauRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("X-Tableau-Auth"); got != "session-token" {
			t.Errorf("X-Tableau-Auth = %q", got)
		}
		if got := request.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		if got := request.Header.Get("X-TADX-Correlation-ID"); got != "correlation-1" {
			t.Errorf("correlation header = %q", got)
		}
		writer.Header().Set("X-Tableau-Request-Id", "tableau-request-1")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `{"ok":true}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", func() string { return "correlation-1" })
	response, err := transport.Do(context.Background(), testSession{token: "session-token"}, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/api/3.29/sites/site-luid/workbooks", Operation: "workbook.list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.TableauRequestID != "tableau-request-1" || string(response.Body) != `{"ok":true}` {
		t.Fatalf("response = %#v", response)
	}
}

func TestTransportReturnsStructuredUpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "request-403")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(writer, `{"error":{"code":"403007","summary":"Publishing forbidden","detail":"No project permission"}}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{Method: http.MethodPost, ServerURL: server.URL, Path: "/publish", Operation: "workbook.publish"})
	upstream, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	if upstream.StatusCode != http.StatusForbidden || upstream.Code != "403007" || upstream.TableauRequestID != "request-403" {
		t.Fatalf("upstream error = %#v", upstream)
	}
	if strings.Contains(upstream.Error(), "session-token") {
		t.Fatalf("error leaked token: %v", upstream)
	}
}

func TestTransportBoundsBufferedResponsesAndPreservesRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "request-large")
		_, _ = io.WriteString(writer, "12345")
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/large", Operation: "workbook.list", MaxResponseBytes: 4,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("error = %v", err)
	}
	if got := RequestID(err); got != "request-large" {
		t.Fatalf("RequestID() = %q", got)
	}
	var status interface{ HTTPStatus() int }
	if !errors.As(err, &status) || status.HTTPStatus() != http.StatusOK {
		t.Fatalf("response status was not preserved: %T %v", err, err)
	}
}

func TestTransportRedactsOverlappingSecretsAtomically(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"code":"401001","summary":"Sign-in failed","detail":"rejected abc123xyz"}}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodPost, ServerURL: server.URL, Path: "/signin", Operation: "auth.check", Secrets: []string{"abc123", "123xyz"},
	})
	upstream, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	if upstream.Detail != "rejected [REDACTED]" || strings.Contains(upstream.Detail, "abc") || strings.Contains(upstream.Detail, "xyz") {
		t.Fatalf("redacted detail = %q", upstream.Detail)
	}
}

func TestTransportRejectsResponseLimitThatWouldOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/large", Operation: "workbook.list", MaxResponseBytes: math.MaxInt64,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid Tableau response limit") {
		t.Fatalf("error = %v", err)
	}
}
