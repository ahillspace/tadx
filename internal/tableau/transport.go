// Package tableau provides the shared released-REST transport contract.
package tableau

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/auth"
)

const (
	defaultAPIVersion          = "3.29"
	defaultMaxResponseBytes    = 256 * 1024 * 1024
	maxUpstreamDiagnosticBytes = 16 * 1024
	// defaultRequestTimeout bounds a single request when the caller supplies a
	// context without its own deadline, so a stalled server cannot hang forever.
	defaultRequestTimeout = 120 * time.Second
)

// Request describes one released Tableau REST request.
type Request struct {
	Method      string
	ServerURL   string
	Path        string
	Query       url.Values
	Header      http.Header
	Body        []byte
	Operation   string
	Secrets     []string
	Accept      string
	ContentType string
	// MaxResponseBytes bounds the buffered body. Zero uses the shared 256 MiB limit.
	MaxResponseBytes int64
}

// Response is a fully read successful Tableau REST response.
type Response struct {
	StatusCode       int
	Header           http.Header
	Body             []byte
	TableauRequestID string
	redact           func(string) string
}

// UpstreamError preserves Tableau's stable error fields.
type UpstreamError struct {
	Operation        string
	StatusCode       int
	Code             string
	Summary          string
	Detail           string
	TableauRequestID string
	retryAfter       time.Duration
	retryAfterSet    bool
}

type responseReadError struct {
	operation        string
	requestID        string
	statusCode       int
	cause            error
	retryable        bool
	correctiveAction string
}

// ProtocolError preserves response context for an invalid successful response.
type ProtocolError struct {
	operation        string
	requestID        string
	statusCode       int
	cause            error
	retryable        bool
	correctiveAction string
}

type redirectError struct{ cause error }

type requestError struct {
	operation        string
	cause            error
	retryable        bool
	correctiveAction string
}

func (e *requestError) Error() string {
	return fmt.Sprintf("Tableau %s request: %v", e.operation, e.cause)
}

func (e *requestError) Unwrap() error            { return e.cause }
func (e *requestError) Retryable() bool          { return e.retryable }
func (e *requestError) CorrectiveAction() string { return e.correctiveAction }

func (e *responseReadError) Error() string {
	return fmt.Sprintf("read Tableau %s response: %v", e.operation, e.cause)
}

func (e *responseReadError) Unwrap() error     { return e.cause }
func (e *responseReadError) RequestID() string { return e.requestID }
func (e *responseReadError) HTTPStatus() int   { return e.statusCode }
func (e *responseReadError) TableauCode() string {
	return ""
}
func (e *responseReadError) TableauSummary() string {
	return ""
}
func (e *responseReadError) TableauDetail() string {
	return ""
}

func (e *responseReadError) Retryable() bool { return e.retryable }
func (e *responseReadError) CorrectiveAction() string {
	return e.correctiveAction
}

// NewProtocolError creates a response-context carrier for protocol validation failures.
func NewProtocolError(operation string, response Response, cause error, retryable bool) *ProtocolError {
	if cause != nil && response.redact != nil {
		cause = errors.New(response.redact(cause.Error()))
	}
	correctiveAction := "Inspect the remote operation outcome before retrying."
	if retryable {
		correctiveAction = "Retry after Tableau returns a complete valid response."
	}
	return &ProtocolError{operation: operation, requestID: response.TableauRequestID, statusCode: response.StatusCode, cause: cause, retryable: retryable, correctiveAction: correctiveAction}
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("invalid Tableau %s response: %v", e.operation, e.cause)
}

func (e *ProtocolError) Unwrap() error            { return e.cause }
func (e *ProtocolError) RequestID() string        { return e.requestID }
func (e *ProtocolError) HTTPStatus() int          { return e.statusCode }
func (e *ProtocolError) TableauCode() string      { return "" }
func (e *ProtocolError) TableauSummary() string   { return "" }
func (e *ProtocolError) TableauDetail() string    { return "" }
func (e *ProtocolError) Retryable() bool          { return e.retryable }
func (e *ProtocolError) CorrectiveAction() string { return e.correctiveAction }
func (e *redirectError) Error() string            { return e.cause.Error() }
func (e *redirectError) Unwrap() error            { return e.cause }

func (e *UpstreamError) Error() string {
	parts := []string{fmt.Sprintf("Tableau request failed with HTTP %d", e.StatusCode)}
	if e.Code != "" {
		parts = append(parts, "code "+e.Code)
	}
	if e.Summary != "" {
		parts = append(parts, e.Summary)
	}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	return strings.Join(parts, ": ")
}

// RequestID exposes the upstream request identifier without coupling actions to the transport.
func (e *UpstreamError) RequestID() string {
	if e == nil {
		return ""
	}
	return e.TableauRequestID
}

// HTTPStatus returns the upstream HTTP status for structured error rendering.
func (e *UpstreamError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}

// TableauCode returns Tableau's stable upstream error code.
func (e *UpstreamError) TableauCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

// TableauSummary returns Tableau's redacted upstream summary.
func (e *UpstreamError) TableauSummary() string {
	if e == nil {
		return ""
	}
	return e.Summary
}

// TableauDetail returns Tableau's redacted upstream detail.
func (e *UpstreamError) TableauDetail() string {
	if e == nil {
		return ""
	}
	return e.Detail
}

func (e *UpstreamError) Retryable() bool {
	if e == nil {
		return false
	}
	retryable, _ := upstreamAdvice(e.StatusCode)
	return retryable
}

func (e *UpstreamError) CorrectiveAction() string {
	if e == nil {
		return ""
	}
	_, correctiveAction := upstreamAdvice(e.StatusCode)
	return correctiveAction
}

// RetryAfter surfaces the upstream Retry-After hint so retrying callers can honor it.
func (e *UpstreamError) RetryAfter() (time.Duration, bool) {
	if e == nil {
		return 0, false
	}
	return e.retryAfter, e.retryAfterSet
}

// RetryAfter returns the first Retry-After hint carried in an error chain.
func RetryAfter(err error) (time.Duration, bool) {
	var carrier interface {
		RetryAfter() (time.Duration, bool)
	}
	if errors.As(err, &carrier) {
		return carrier.RetryAfter()
	}
	return 0, false
}

// RequestID returns the first upstream request identifier in an error chain.
func RequestID(err error) string {
	var carrier interface{ RequestID() string }
	if errors.As(err, &carrier) {
		return carrier.RequestID()
	}
	return ""
}

// Transport applies standard headers, authorization, response capture, and errors.
type Transport struct {
	client         *http.Client
	apiVersion     string
	correlationID  func() string
	requestTimeout time.Duration
}

// NewTransport creates the shared Tableau REST transport.
func NewTransport(client *http.Client, apiVersion string, correlationID func() string) *Transport {
	if client == nil {
		client = http.DefaultClient
	}
	clientCopy := *client
	previousCheckRedirect := clientCopy.CheckRedirect
	clientCopy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > 0 {
			previous := via[len(via)-1].URL
			if strings.EqualFold(previous.Scheme, "https") && !strings.EqualFold(request.URL.Scheme, "https") {
				return &redirectError{cause: fmt.Errorf("refusing Tableau redirect from %s to less secure scheme %s", previous.Scheme, request.URL.Scheme)}
			}
			if !sameOrigin(via[0].URL, request.URL) {
				return &redirectError{cause: fmt.Errorf("refusing cross-origin Tableau redirect from %s to %s", via[0].URL.Host, request.URL.Host)}
			}
		}
		if previousCheckRedirect != nil {
			if err := previousCheckRedirect(request, via); err != nil {
				return &redirectError{cause: err}
			}
			return nil
		}
		if len(via) >= 10 {
			return &redirectError{cause: errors.New("stopped after 10 redirects")}
		}
		return nil
	}
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	return &Transport{client: &clientCopy, apiVersion: apiVersion, correlationID: correlationID, requestTimeout: defaultRequestTimeout}
}

// APIVersion returns the configured optimistic REST API version.
func (t *Transport) APIVersion() string { return t.apiVersion }

// SetRequestTimeout overrides the default per-request timeout applied when the
// caller's context carries no deadline. A non-positive value disables the net.
func (t *Transport) SetRequestTimeout(timeout time.Duration) {
	if t == nil {
		return
	}
	t.requestTimeout = timeout
}

// Do performs one request without automatic retries.
func (t *Transport) Do(ctx context.Context, session auth.Session, input Request) (Response, error) {
	if t == nil || t.client == nil {
		return Response{}, errors.New("Tableau transport is not configured")
	}
	maxResponseBytes, err := responseLimit(input.MaxResponseBytes)
	if err != nil {
		return Response{}, err
	}
	base, err := url.Parse(strings.TrimRight(input.ServerURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return Response{}, fmt.Errorf("invalid Tableau server URL %q", input.ServerURL)
	}
	// Never attach a session credential over cleartext. This mirrors the sign-in
	// HTTPS check so the X-Tableau-Auth token can't be sent in the clear.
	if session != nil && !strings.EqualFold(base.Scheme, "https") {
		return Response{}, fmt.Errorf("refusing to send Tableau session credentials to non-HTTPS server URL %q", input.ServerURL)
	}
	path, err := url.Parse(input.Path)
	if err != nil {
		return Response{}, fmt.Errorf("invalid Tableau request path: %w", err)
	}
	basePath := strings.TrimRight(base.Path, "/")
	baseEscaped := strings.TrimRight(base.EscapedPath(), "/")
	base.Path = basePath + "/" + strings.TrimLeft(path.Path, "/")
	// Preserve the already percent-encoded request path so reserved characters
	// (e.g. an escaped slash in a LUID segment) round-trip instead of decoding.
	base.RawPath = baseEscaped + "/" + strings.TrimLeft(path.EscapedPath(), "/")
	base.RawQuery = input.Query.Encode()
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && t.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.requestTimeout)
		defer cancel()
	}
	request, err := http.NewRequestWithContext(ctx, input.Method, base.String(), bytes.NewReader(input.Body))
	if err != nil {
		return Response{}, fmt.Errorf("create Tableau request: %w", err)
	}
	for name, values := range input.Header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	accept := input.Accept
	if accept == "" {
		accept = "application/json"
	}
	request.Header.Set("Accept", accept)
	if input.ContentType != "" {
		request.Header.Set("Content-Type", input.ContentType)
	}
	if t.correlationID != nil {
		if correlation := t.correlationID(); correlation != "" {
			request.Header.Set("X-TADX-Correlation-ID", correlation)
		}
	}
	if session != nil {
		session.Authorize(request)
	}
	effectiveSecrets := append([]string(nil), input.Secrets...)
	effectiveSecrets = append(effectiveSecrets, request.Header.Values(auth.TableauAuthHeader)...)
	response, err := t.client.Do(request)
	if err != nil {
		retryable, correctiveAction := requestAdvice(ctx, input.Method, input.Operation, err)
		return Response{}, &requestError{operation: input.Operation, cause: redact(err, effectiveSecrets), retryable: retryable, correctiveAction: correctiveAction}
	}
	defer response.Body.Close()
	requestID := sanitizeDiagnostic(tableauRequestID(response.Header), effectiveSecrets)
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		retryable, correctiveAction := responseReadAdvice(ctx, input.Method, input.Operation, response.StatusCode)
		return Response{}, &responseReadError{operation: input.Operation, requestID: requestID, statusCode: response.StatusCode, cause: redact(err, effectiveSecrets), retryable: retryable, correctiveAction: correctiveAction}
	}
	if int64(len(body)) > maxResponseBytes {
		return Response{}, &responseReadError{operation: input.Operation, requestID: requestID, statusCode: response.StatusCode, cause: fmt.Errorf("body exceeded %d-byte limit", maxResponseBytes), correctiveAction: "Reduce the response size or use a bounded streaming workflow."}
	}
	redactionSecrets := append([]string(nil), effectiveSecrets...)
	result := Response{
		StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: body, TableauRequestID: requestID,
		redact: func(value string) string { return sanitizeDiagnostic(value, redactionSecrets) },
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return result, nil
	}
	code, summary, detail := parseError(body, effectiveSecrets)
	retryAfter, retryAfterSet := parseRetryAfter(response.Header)
	return Response{}, &UpstreamError{
		Operation: input.Operation, StatusCode: response.StatusCode, Code: code,
		Summary: summary, Detail: detail, TableauRequestID: requestID,
		retryAfter: retryAfter, retryAfterSet: retryAfterSet,
	}
}

// parseRetryAfter interprets the Retry-After header as delta-seconds or an HTTP
// date so retryable classification can surface the upstream backoff hint.
func parseRetryAfter(header http.Header) (time.Duration, bool) {
	value := strings.TrimSpace(header.Get("Retry-After"))
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds < 0 {
			return 0, false
		}
		return time.Duration(seconds) * time.Second, true
	}
	if when, err := http.ParseTime(value); err == nil {
		delay := time.Until(when)
		if delay < 0 {
			delay = 0
		}
		return delay, true
	}
	return 0, false
}

func requestAdvice(ctx context.Context, method, operation string, err error) (bool, string) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false, "Run the operation again only if the cancellation was intentional and the remote outcome is known."
	}
	var redirect *redirectError
	if errors.As(err, &redirect) {
		return false, "Verify the Tableau server URL and redirect policy before retrying."
	}
	if retrySafe(method, operation) {
		return true, "Verify network, DNS, proxy, and TLS connectivity, then retry."
	}
	return false, "Inspect the remote operation outcome before retrying."
}

func responseReadAdvice(ctx context.Context, method, operation string, status int) (bool, string) {
	if ctx.Err() != nil {
		return false, "Run the operation again only if the cancellation was intentional and the remote outcome is known."
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return upstreamAdvice(status)
	}
	if retrySafe(method, operation) {
		return true, "Retry after Tableau returns a complete response."
	}
	return false, "Inspect the remote operation outcome before retrying."
}

func retrySafe(method, operation string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions || operation == "auth.check"
}

// upstreamAdvice classifies an upstream status as retryable. When a retryable
// response also carries a Retry-After header, UpstreamError.RetryAfter exposes
// the backoff hint so retrying callers (e.g. the publish poll loop) can honor it.
func upstreamAdvice(status int) (bool, string) {
	switch {
	case status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests:
		return true, "Wait for the upstream service, then retry."
	case status >= http.StatusInternalServerError:
		return true, "Retry after Tableau or the intermediary service recovers."
	case status == http.StatusUnauthorized:
		return false, "Verify the PAT credentials, expiration, and selected site."
	case status == http.StatusForbidden:
		return false, "Verify the PAT permissions and selected site."
	case status == http.StatusNotFound:
		return false, "Verify the server URL, site content URL, and REST API version."
	default:
		return false, "Review the upstream status and Tableau error details before retrying."
	}
}

func responseLimit(requested int64) (int64, error) {
	if requested == 0 {
		return defaultMaxResponseBytes, nil
	}
	if requested < 0 || requested > defaultMaxResponseBytes {
		return 0, fmt.Errorf("invalid Tableau response limit %d: maximum is %d bytes", requested, defaultMaxResponseBytes)
	}
	return requested, nil
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		originPort(left) == originPort(right)
}

func originPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func tableauRequestID(header http.Header) string {
	for _, name := range []string{"X-Tableau-Request-Id", "X-Request-Id", "Request-Id"} {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func parseError(body []byte, secrets []string) (string, string, string) {
	var jsonEnvelope struct {
		Error struct {
			Code    string `json:"code"`
			Summary string `json:"summary"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &jsonEnvelope) == nil && (jsonEnvelope.Error.Code != "" || jsonEnvelope.Error.Summary != "") {
		return sanitizeDiagnostic(jsonEnvelope.Error.Code, secrets), sanitizeDiagnostic(jsonEnvelope.Error.Summary, secrets), sanitizeDiagnostic(jsonEnvelope.Error.Detail, secrets)
	}
	var xmlEnvelope struct {
		Error struct {
			Code    string `xml:"code,attr"`
			Summary string `xml:"summary"`
			Detail  string `xml:"detail"`
		} `xml:"error"`
	}
	if xml.Unmarshal(body, &xmlEnvelope) == nil && (xmlEnvelope.Error.Code != "" || xmlEnvelope.Error.Summary != "") {
		return sanitizeDiagnostic(xmlEnvelope.Error.Code, secrets), sanitizeDiagnostic(xmlEnvelope.Error.Summary, secrets), sanitizeDiagnostic(xmlEnvelope.Error.Detail, secrets)
	}
	return "", "Upstream request failed", sanitizeDiagnostic(string(body), secrets)
}

func sanitizeDiagnostic(value string, secrets []string) string {
	value = strings.TrimSpace(auth.Redact(strings.ToValidUTF8(value, "\uFFFD"), secrets...))
	if len(value) <= maxUpstreamDiagnosticBytes {
		return value
	}
	const suffix = "\n[truncated]"
	limit := maxUpstreamDiagnosticBytes - len(suffix)
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return strings.TrimSpace(value[:limit]) + suffix
}

func redact(err error, secrets []string) error {
	if err == nil {
		return nil
	}
	return errors.New(sanitizeDiagnostic(err.Error(), secrets))
}

// Page is the normalized classic REST pagination envelope.
type Page struct {
	Number int `json:"page_number"`
	Size   int `json:"page_size"`
	Total  int `json:"total"`
}
