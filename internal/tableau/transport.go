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
	"strings"

	"github.com/ahillspace/tadx/internal/auth"
)

const (
	defaultAPIVersion       = "3.29"
	defaultMaxResponseBytes = 256 * 1024 * 1024
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
}

// UpstreamError preserves Tableau's stable error fields.
type UpstreamError struct {
	Operation        string
	StatusCode       int
	Code             string
	Summary          string
	Detail           string
	TableauRequestID string
}

type responseReadError struct {
	operation  string
	requestID  string
	statusCode int
	cause      error
}

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
	client        *http.Client
	apiVersion    string
	correlationID func() string
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
				return fmt.Errorf("refusing Tableau redirect from %s to less secure scheme %s", previous.Scheme, request.URL.Scheme)
			}
			if !sameOrigin(via[0].URL, request.URL) {
				return fmt.Errorf("refusing cross-origin Tableau redirect from %s to %s", via[0].URL.Host, request.URL.Host)
			}
		}
		if previousCheckRedirect != nil {
			return previousCheckRedirect(request, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	if apiVersion == "" {
		apiVersion = defaultAPIVersion
	}
	return &Transport{client: &clientCopy, apiVersion: apiVersion, correlationID: correlationID}
}

// APIVersion returns the configured optimistic REST API version.
func (t *Transport) APIVersion() string { return t.apiVersion }

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
	path, err := url.Parse(input.Path)
	if err != nil {
		return Response{}, fmt.Errorf("invalid Tableau request path: %w", err)
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/" + strings.TrimLeft(path.Path, "/")
	base.RawQuery = input.Query.Encode()
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
		return Response{}, fmt.Errorf("Tableau %s request: %w", input.Operation, redact(err, effectiveSecrets))
	}
	defer response.Body.Close()
	requestID := redactText(tableauRequestID(response.Header), effectiveSecrets)
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Response{}, &responseReadError{operation: input.Operation, requestID: requestID, statusCode: response.StatusCode, cause: redact(err, effectiveSecrets)}
	}
	if int64(len(body)) > maxResponseBytes {
		return Response{}, &responseReadError{operation: input.Operation, requestID: requestID, statusCode: response.StatusCode, cause: fmt.Errorf("body exceeded %d-byte limit", maxResponseBytes)}
	}
	result := Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: body, TableauRequestID: requestID}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return result, nil
	}
	code, summary, detail := parseError(body)
	return Response{}, &UpstreamError{
		Operation: input.Operation, StatusCode: response.StatusCode, Code: redactText(code, effectiveSecrets),
		Summary: redactText(summary, effectiveSecrets), Detail: redactText(detail, effectiveSecrets), TableauRequestID: requestID,
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

func parseError(body []byte) (string, string, string) {
	var jsonEnvelope struct {
		Error struct {
			Code    string `json:"code"`
			Summary string `json:"summary"`
			Detail  string `json:"detail"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &jsonEnvelope) == nil && (jsonEnvelope.Error.Code != "" || jsonEnvelope.Error.Summary != "") {
		return jsonEnvelope.Error.Code, jsonEnvelope.Error.Summary, jsonEnvelope.Error.Detail
	}
	var xmlEnvelope struct {
		Error struct {
			Code    string `xml:"code,attr"`
			Summary string `xml:"summary"`
			Detail  string `xml:"detail"`
		} `xml:"error"`
	}
	if xml.Unmarshal(body, &xmlEnvelope) == nil && (xmlEnvelope.Error.Code != "" || xmlEnvelope.Error.Summary != "") {
		return xmlEnvelope.Error.Code, strings.TrimSpace(xmlEnvelope.Error.Summary), strings.TrimSpace(xmlEnvelope.Error.Detail)
	}
	return "", "Upstream request failed", strings.TrimSpace(string(body))
}

func redact(err error, secrets []string) error {
	if err == nil {
		return nil
	}
	return errors.New(redactText(err.Error(), secrets))
}

func redactText(value string, secrets []string) string {
	return auth.Redact(value, secrets...)
}

// Page is the normalized classic REST pagination envelope.
type Page struct {
	Number int `json:"page_number"`
	Size   int `json:"page_size"`
	Total  int `json:"total"`
}
