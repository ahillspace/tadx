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
