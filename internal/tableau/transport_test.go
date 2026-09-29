package tableau

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
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

func TestTransportClassifiesOversizedResponseAsNonretryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "oversized")
	}))
	defer server.Close()
	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/large", Operation: "workbook.list", MaxResponseBytes: 1})
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || advice.Retryable() {
		t.Fatalf("retry advice = %#v, error = %v", advice, err)
	}
}

func TestTransportRejectsCrossOriginRedirectBeforeReplayingCredentials(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetRequests.Add(1) }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
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
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || advice.Retryable() {
		t.Fatalf("retry advice = %#v", advice)
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
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || advice.Retryable() {
		t.Fatalf("retry advice = %#v", advice)
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
		{operation: "metadata.query", want: true},
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

func TestTransportClassifiesResponseReadRetrySafety(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		method    string
		operation string
		status    int
		want      bool
	}{
		{name: "workbook read", method: http.MethodGet, operation: "workbook.list", status: http.StatusOK, want: true},
		{name: "authentication", method: http.MethodPost, operation: "auth.check", status: http.StatusOK, want: true},
		{name: "metadata query", method: http.MethodPost, operation: "metadata.query", status: http.StatusOK, want: true},
		{name: "publish mutation", method: http.MethodPost, operation: "workbook.publish", status: http.StatusOK, want: false},
		{name: "unauthorized authentication", method: http.MethodPost, operation: "auth.check", status: http.StatusUnauthorized, want: false},
		{name: "unavailable workbook read", method: http.MethodGet, operation: "workbook.list", status: http.StatusServiceUnavailable, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := NewTransport(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Header: make(http.Header), Body: errorReader{err: io.ErrUnexpectedEOF}}, nil
			})}, "3.29", nil)
			_, err := transport.Do(context.Background(), nil, Request{Method: test.method, ServerURL: "https://tableau.example", Path: "/request", Operation: test.operation})
			var advice interface {
				Retryable() bool
				CorrectiveAction() string
			}
			if !errors.As(err, &advice) || advice.Retryable() != test.want || advice.CorrectiveAction() == "" {
				t.Fatalf("retry advice = %#v, error = %v", advice, err)
			}
		})
	}
}

func TestTransportClassifiesCancellationAsNonretryable(t *testing.T) {
	tests := []struct {
		name   string
		client *http.Client
		url    string
		ctx    context.Context
	}{
		{
			name: "cancellation",
			client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				return nil, request.Context().Err()
			})},
			url: "https://tableau.example",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := NewTransport(test.client, "3.29", nil)
			_, err := transport.Do(test.ctx, nil, Request{Method: http.MethodPost, ServerURL: test.url, Path: "/signin", Operation: "auth.check"})
			var advice interface{ Retryable() bool }
			if !errors.As(err, &advice) || advice.Retryable() {
				t.Fatalf("retry advice = %#v, error = %v", advice, err)
			}
		})
	}
}

func TestTransportBoundsUnstructuredUpstreamDiagnostic(t *testing.T) {
	body := bytes.Repeat([]byte("x"), maxUpstreamDiagnosticBytes*2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write(body)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list"})
	var upstream *UpstreamError
	if !errors.As(err, &upstream) || len(upstream.Detail) > maxUpstreamDiagnosticBytes || !strings.HasSuffix(upstream.Detail, "[truncated]") {
		t.Fatalf("upstream error = %#v", upstream)
	}
}

func TestTransportRedactsSecretCrossingDiagnosticBoundary(t *testing.T) {
	secret := "boundary-secret-value"
	prefix := bytes.Repeat([]byte("x"), maxUpstreamDiagnosticBytes-len("\n[truncated]")-len(secret)/2)
	body := append(prefix, []byte(secret)...)
	body = append(body, bytes.Repeat([]byte("y"), maxUpstreamDiagnosticBytes)...)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write(body)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list", Secrets: []string{secret}})
	var upstream *UpstreamError
	if !errors.As(err, &upstream) {
		t.Fatalf("error = %T %v", err, err)
	}
	if strings.Contains(upstream.Detail, secret) || strings.Contains(upstream.Detail, secret[:len(secret)/2]) || len(upstream.Detail) > maxUpstreamDiagnosticBytes || !strings.HasSuffix(upstream.Detail, "[truncated]") {
		t.Fatalf("upstream detail = %q", upstream.Detail)
	}
}

func TestTransportSanitizesStructuredUpstreamDiagnostics(t *testing.T) {
	const secret = "structured-secret"
	long := secret + strings.Repeat("x", maxUpstreamDiagnosticBytes*2)
	for _, test := range []struct {
		name        string
		contentType string
		body        string
	}{
		{name: "json", contentType: "application/json", body: `{"error":{"code":"` + long + `","summary":"` + long + `","detail":"` + long + `"}}`},
		{name: "xml", contentType: "application/xml", body: `<tsResponse><error code="` + long + `"><summary>` + long + `</summary><detail>` + long + `</detail></error></tsResponse>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Type", test.contentType)
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, test.body)
			}))
			defer server.Close()

			transport := NewTransport(server.Client(), "3.29", nil)
			_, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list", Secrets: []string{secret}})
			var upstream *UpstreamError
			if !errors.As(err, &upstream) {
				t.Fatalf("error = %T %v", err, err)
			}
			for name, value := range map[string]string{"code": upstream.Code, "summary": upstream.Summary, "detail": upstream.Detail} {
				if len(value) > maxUpstreamDiagnosticBytes || strings.Contains(value, secret) || !strings.HasSuffix(value, "[truncated]") {
					t.Errorf("%s was not sanitized: length=%d suffix=%q", name, len(value), value[max(0, len(value)-20):])
				}
			}
		})
	}
}

func TestTransportSanitizesRequestID(t *testing.T) {
	const secret = "request-secret"
	requestID := secret + strings.Repeat("r", maxUpstreamDiagnosticBytes*2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-Tableau-Request-Id", requestID)
		_, _ = io.WriteString(writer, `{}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	response, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list", Secrets: []string{secret}})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.TableauRequestID) > maxUpstreamDiagnosticBytes || strings.Contains(response.TableauRequestID, secret) || !strings.HasSuffix(response.TableauRequestID, "[truncated]") {
		t.Fatalf("request ID was not sanitized: length=%d", len(response.TableauRequestID))
	}
}

func TestTransportRejectsPlaintextAuthenticatedRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), testSession{token: "session-token"}, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "session-token") {
		t.Fatalf("error leaked token: %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func TestTransportStallTimeoutBoundsMissingResponseHeaders(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	transport := NewTransport(server.Client(), "3.29", nil)
	transport.SetStallTimeout(20 * time.Millisecond)
	started := time.Now()
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/hang", Operation: "workbook.list",
	})
	if err == nil || !strings.Contains(err.Error(), "no response headers within 20ms") {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("request was not bounded by the stall timeout: %s", elapsed)
	}
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || !advice.Retryable() {
		t.Fatalf("a stalled read should be retryable: %#v", advice)
	}
}

func TestTransportCompletesSlowButSteadyResponseBody(t *testing.T) {
	const chunks = 12
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		for range chunks {
			_, _ = writer.Write(bytes.Repeat([]byte("x"), 1024))
			writer.(http.Flusher).Flush()
			time.Sleep(25 * time.Millisecond)
		}
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	transport.SetStallTimeout(200 * time.Millisecond)
	started := time.Now()
	response, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "workbook.pull",
	})
	if err != nil {
		t.Fatalf("a progressing download failed after %s: %v", time.Since(started), err)
	}
	if len(response.Body) != chunks*1024 {
		t.Fatalf("body length = %d", len(response.Body))
	}
	if elapsed := time.Since(started); elapsed <= 200*time.Millisecond {
		t.Fatalf("download finished in %s, so it did not outlast the stall timeout", elapsed)
	}
}

func TestTransportIgnoresWholeRequestClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		for range 8 {
			_, _ = writer.Write([]byte("chunk"))
			writer.(http.Flusher).Flush()
			time.Sleep(25 * time.Millisecond)
		}
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 50 * time.Millisecond
	transport := NewTransport(client, "3.29", nil)
	transport.SetStallTimeout(200 * time.Millisecond)
	if _, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "workbook.pull",
	}); err != nil {
		t.Fatalf("a whole-request client timeout cut off a progressing download: %v", err)
	}
	if client.Timeout != 50*time.Millisecond {
		t.Fatalf("NewTransport modified the caller's client: %s", client.Timeout)
	}
}

func TestTransportStallTimeoutBoundsStalledResponseBody(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("partial"))
		writer.(http.Flusher).Flush()
		<-release
	}))
	defer server.Close()
	defer close(release)

	transport := NewTransport(server.Client(), "3.29", nil)
	transport.SetStallTimeout(50 * time.Millisecond)
	started := time.Now()
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "workbook.pull",
	})
	if err == nil || !strings.Contains(err.Error(), "read Tableau workbook.pull response: no response body progress for 50ms") {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("stalled body was not bounded: %s", elapsed)
	}
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || !advice.Retryable() {
		t.Fatalf("a stalled read should be retryable: %#v", advice)
	}
}

// slowUploadTransport reads the request body in small paced chunks, standing in
// for a slow network link without depending on kernel socket buffer sizes.
func slowUploadTransport(chunk int, pause time.Duration, stallAfter int) roundTripFunc {
	return func(request *http.Request) (*http.Response, error) {
		buffer := make([]byte, chunk)
		for reads := 0; ; reads++ {
			if stallAfter >= 0 && reads == stallAfter {
				<-request.Context().Done()
				return nil, request.Context().Err()
			}
			if _, err := request.Body.Read(buffer); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return nil, err
			}
			select {
			case <-request.Context().Done():
				return nil, request.Context().Err()
			case <-time.After(pause):
			}
		}
		_ = request.Body.Close()
		return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}"))}, nil
	}
}

func TestTransportCompletesSlowButSteadyRequestBody(t *testing.T) {
	transport := NewTransport(&http.Client{Transport: slowUploadTransport(1024, 20*time.Millisecond, -1)}, "3.29", nil)
	transport.SetStallTimeout(150 * time.Millisecond)
	started := time.Now()
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodPut, ServerURL: "https://tableau.example", Path: "/fileUploads/id",
		Operation: "workbook.publish.upload", Body: bytes.Repeat([]byte("u"), 16*1024),
	})
	if err != nil {
		t.Fatalf("a progressing upload failed after %s: %v", time.Since(started), err)
	}
	if elapsed := time.Since(started); elapsed <= 150*time.Millisecond {
		t.Fatalf("upload finished in %s, so it did not outlast the stall timeout", elapsed)
	}
}

func TestTransportStallTimeoutBoundsStalledRequestBody(t *testing.T) {
	transport := NewTransport(&http.Client{Transport: slowUploadTransport(1024, time.Millisecond, 2)}, "3.29", nil)
	transport.SetStallTimeout(50 * time.Millisecond)
	started := time.Now()
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodPut, ServerURL: "https://tableau.example", Path: "/fileUploads/id",
		Operation: "workbook.publish.upload", Body: bytes.Repeat([]byte("u"), 16*1024),
	})
	if err == nil || !strings.Contains(err.Error(), "Tableau workbook.publish.upload request: no request body progress for 50ms") {
		t.Fatalf("error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("stalled upload was not bounded: %s", elapsed)
	}
	var advice interface {
		Retryable() bool
		CorrectiveAction() string
	}
	if !errors.As(err, &advice) || advice.Retryable() || advice.CorrectiveAction() != "Inspect the remote operation outcome before retrying." {
		t.Fatalf("a stalled mutation must not be retryable: %#v", advice)
	}
	if !SubmissionAttempted(err) {
		t.Fatal("a stalled upload must report that submission was attempted")
	}
}

func TestTransportPreservesRequestBodyLengthAndReplay(t *testing.T) {
	var lengths []int64
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		lengths = append(lengths, request.ContentLength)
		body, _ := io.ReadAll(request.Body)
		bodies = append(bodies, string(body))
		if request.URL.Path == "/first" {
			http.Redirect(writer, request, "/second", http.StatusTemporaryRedirect)
		}
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	if _, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodPost, ServerURL: server.URL, Path: "/first", Operation: "auth.check", Body: []byte("payload"),
	}); err != nil {
		t.Fatal(err)
	}
	if len(lengths) != 2 || lengths[0] != 7 || lengths[1] != 7 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("lengths = %v, bodies = %q", lengths, bodies)
	}
}

func TestTransportCallerCancellationDuringBodyIsNotAStall(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("partial"))
		writer.(http.Flusher).Flush()
		<-release
	}))
	defer server.Close()
	defer close(release)

	transport := NewTransport(server.Client(), "3.29", nil)
	transport.SetStallTimeout(time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := transport.Do(ctx, nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "workbook.pull",
	})
	if err == nil || strings.Contains(err.Error(), "progress") {
		t.Fatalf("error = %v", err)
	}
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || advice.Retryable() {
		t.Fatalf("caller cancellation must stay nonretryable: %#v", advice)
	}
}

func TestTransportPreservesEscapedRequestPath(t *testing.T) {
	var escapedPath string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		escapedPath = request.URL.EscapedPath()
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/api/3.29/sites/site/workbooks/wb%2Fslash", Operation: "workbook.list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(escapedPath, "%2F") {
		t.Fatalf("escaped request path was not preserved: %q", escapedPath)
	}
}

func TestUpstreamErrorSurfacesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "2")
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"error":{"code":"503000","summary":"Unavailable"}}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	_, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list"})
	wait, ok := RetryAfter(err)
	if !ok || wait != 2*time.Second {
		t.Fatalf("RetryAfter() = %s, %v", wait, ok)
	}
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || !advice.Retryable() {
		t.Fatalf("retry advice = %#v", advice)
	}
}

func TestProtocolErrorSanitizesCause(t *testing.T) {
	const secret = "protocol-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{}`)
	}))
	defer server.Close()

	transport := NewTransport(server.Client(), "3.29", nil)
	response, err := transport.Do(context.Background(), nil, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/workbooks", Operation: "workbook.list", Secrets: []string{secret}})
	if err != nil {
		t.Fatal(err)
	}
	protocolErr := NewProtocolError("workbook.list", response, errors.New(secret+strings.Repeat("p", maxUpstreamDiagnosticBytes*2)), true)
	cause := errors.Unwrap(protocolErr)
	if cause == nil || len(cause.Error()) > maxUpstreamDiagnosticBytes || strings.Contains(cause.Error(), secret) || !strings.HasSuffix(cause.Error(), "[truncated]") {
		t.Fatalf("protocol cause was not sanitized: %v", cause)
	}
}
