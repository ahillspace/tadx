package tableau

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type testSession struct{ token string }

func (s testSession) Authorize(request *http.Request) { request.Header.Set("X-Tableau-Auth", s.token) }
func (s testSession) SiteLUID() string                { return "site-luid" }
func (s testSession) UserLUID() string                { return "user-luid" }
func (s testSession) String() string                  { return "redacted session" }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

func (errorReader) Close() error { return nil }

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
	if upstream.Detail != "rejected *********" || strings.Contains(upstream.Detail, "abc") || strings.Contains(upstream.Detail, "xyz") {
		t.Fatalf("redacted detail = %q", upstream.Detail)
	}
}

func TestTransportRedactsAuthorizedSessionTokenFromUpstreamCarriers(t *testing.T) {
	const token = "session-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", "request-"+token)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"code":"code-session-token","summary":"summary session-token","detail":"detail session-token"}}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), testSession{token: token}, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list",
	})
	upstream, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error = %T %v", err, err)
	}
	for name, value := range map[string]string{
		"error": upstream.Error(), "code": upstream.Code, "summary": upstream.Summary,
		"detail": upstream.Detail, "request ID": upstream.TableauRequestID,
	} {
		if strings.Contains(value, token) || !strings.Contains(value, "[REDACTED]") {
			t.Errorf("%s leaked authorized session token: %q", name, value)
		}
	}
}

func TestTransportRedactsAuthorizedSessionTokenFromRequestAndResponseReadErrors(t *testing.T) {
	const token = "session-token"
	tests := []struct {
		name      string
		transport http.RoundTripper
		requestID string
	}{
		{
			name: "request",
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("request exposed " + token)
			}),
		},
		{
			name: "response read",
			transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"X-Tableau-Request-Id": []string{"request-" + token}},
					Body:       errorReader{err: errors.New("read exposed " + token)},
				}, nil
			}),
			requestID: "request-[REDACTED]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := NewTransport(&http.Client{Transport: test.transport}, "3.29", nil)
			_, err := transport.Do(context.Background(), testSession{token: token}, Request{
				Method: http.MethodGet, ServerURL: "https://tableau.example", Path: "/workbooks", Operation: "workbook.list",
			})
			if err == nil || strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[REDACTED]") {
				t.Fatalf("error leaked authorized session token: %v", err)
			}
			if got := RequestID(err); got != test.requestID {
				t.Fatalf("RequestID() = %q, want %q", got, test.requestID)
			}
		})
	}
}

func TestTransportRejectsResponseLimitAboveSharedCeilingBeforeRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/large", Operation: "workbook.list", MaxResponseBytes: defaultMaxResponseBytes + 1,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid Tableau response limit") {
		t.Fatalf("error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestTransportRejectsCrossOriginRedirectBeforeReplayingCredentials(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetRequests.Add(1) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/signin", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	transport := NewTransport(source.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), testSession{token: "session-token"}, Request{
		Method: http.MethodPost, ServerURL: source.URL, Path: "/signin", Operation: "auth.check", Body: []byte(`{"secret":"pat-secret"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "cross-origin Tableau redirect") {
		t.Fatalf("error = %v", err)
	}
	if targetRequests.Load() != 0 {
		t.Fatalf("redirect target requests = %d, want 0", targetRequests.Load())
	}
}

func TestTransportRejectsSchemeDowngradeRedirect(t *testing.T) {
	var source *httptest.Server
	source = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		location := strings.Replace(source.URL, "https://", "http://", 1) + "/signin"
		http.Redirect(writer, request, location, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	transport := NewTransport(source.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), testSession{token: "session-token"}, Request{
		Method: http.MethodPost, ServerURL: source.URL, Path: "/signin", Operation: "auth.check", Body: []byte(`{"secret":"pat-secret"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "less secure scheme") {
		t.Fatalf("error = %v", err)
	}
}

func TestTransportClassifiesRequestRetrySafety(t *testing.T) {
	t.Parallel()

	transport := NewTransport(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})}, "3.29", nil)
	for _, test := range []struct {
		operation string
		want      bool
	}{
		{operation: "auth.check", want: true},
		{operation: "workbook.publish", want: false},
	} {
		_, err := transport.Do(context.Background(), nil, Request{
			Method: http.MethodPost, ServerURL: "https://tableau.example", Path: "/request", Operation: test.operation,
		})
		var advice interface {
			Retryable() bool
			CorrectiveAction() string
		}
		if !errors.As(err, &advice) || advice.Retryable() != test.want || advice.CorrectiveAction() == "" {
			t.Fatalf("%s retry advice = %#v, error = %v", test.operation, advice, err)
		}
	}
}
