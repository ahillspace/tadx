package tableau

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNegotiateAPIVersionUsesUnauthenticatedXMLBaseline(t *testing.T) {
	for _, test := range []struct{ serverVersion, want string }{
		{"3.29", "3.29"}, {"3.30", "3.29"}, {"4.0", "3.29"},
		{"3.9", "3.9"}, {"3.16", "3.16"}, {"3.6", "3.6"},
	} {
		t.Run(test.serverVersion, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/tableau/api/2.4/serverinfo" || r.Header.Get("Accept") != "application/xml" {
					t.Errorf("discovery request = %s %s, Accept %q", r.Method, r.URL.Path, r.Header.Get("Accept"))
				}
				if r.Header.Get("X-Tableau-Auth") != "" || r.Header.Get("Authorization") != "" || r.ContentLength != 0 {
					t.Error("discovery sent authorization or a request body")
				}
				_, _ = io.WriteString(w, `<tsResponse xmlns="http://tableau.com/api"><serverInfo><productVersion build="example-build">example-release</productVersion><restApiVersion>`+test.serverVersion+`</restApiVersion></serverInfo></tsResponse>`)
			}))
			t.Cleanup(server.Close)
			transport := NewTransport(server.Client(), "", nil)
			if transport.APIVersion() != "" {
				t.Fatal("transport selected a version without server evidence")
			}
			if err := transport.NegotiateAPIVersion(t.Context(), server.URL+"/tableau"); err != nil {
				t.Fatal(err)
			}
			if got := transport.APIVersion(); got != test.want {
				t.Fatalf("negotiated version = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNegotiateAPIVersionRejectsMissingMalformedAndDuplicatedEvidence(t *testing.T) {
	for name, body := range map[string]string{
		"missing info":       `<tsResponse/>`,
		"missing version":    `<tsResponse><serverInfo/></tsResponse>`,
		"wrong root":         `<other><serverInfo><restApiVersion>3.29</restApiVersion></serverInfo></other>`,
		"broken XML":         `<tsResponse><serverInfo><restApiVersion>REJECTED_VALUE</serverInfo></tsResponse>`,
		"invalid version":    `<tsResponse><serverInfo><restApiVersion>REJECTED_VALUE</restApiVersion></serverInfo></tsResponse>`,
		"version suffix":     `<tsResponse><serverInfo><restApiVersion>3.29.REJECTED_VALUE</restApiVersion></serverInfo></tsResponse>`,
		"signed version":     `<tsResponse><serverInfo><restApiVersion>+3.29</restApiVersion></serverInfo></tsResponse>`,
		"version whitespace": `<tsResponse><serverInfo><restApiVersion> 3.29 </restApiVersion></serverInfo></tsResponse>`,
		"duplicate version":  `<tsResponse><serverInfo><restApiVersion>3.6</restApiVersion><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse>`,
		"duplicate info":     `<tsResponse><serverInfo><restApiVersion>3.6</restApiVersion></serverInfo><serverInfo><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse>`,
		"multiple documents": `<tsResponse><serverInfo><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse><REJECTED_VALUE/>`,
		"trailing text":      `<tsResponse><serverInfo><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse>REJECTED_VALUE`,
		"overflow":           `<tsResponse><serverInfo><restApiVersion>99999999999999999999999999999.29</restApiVersion></serverInfo></tsResponse>`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Tableau-Request-Id", "discovery-invalid")
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(server.Close)
			transport := NewTransport(server.Client(), "", nil)
			err := transport.NegotiateAPIVersion(t.Context(), server.URL)
			protocol, ok := errors.AsType[*ProtocolError](err)
			if !ok || protocol.HTTPStatus() != 200 || protocol.RequestID() != "discovery-invalid" || strings.Contains(err.Error(), "REJECTED_VALUE") {
				t.Fatalf("discovery error does not preserve safe protocol context: %T %v", err, err)
			}
			if transport.APIVersion() != "" {
				t.Fatal("invalid discovery selected a version")
			}
		})
	}
}

func TestNegotiateAPIVersionRejectsServersBelowPATMinimum(t *testing.T) {
	for _, version := range []string{"2.4", "3.0", "3.5"} {
		t.Run(version, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Tableau-Request-Id", "discovery-unsupported")
				_, _ = io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>`+version+`</restApiVersion></serverInfo></tsResponse>`)
			}))
			t.Cleanup(server.Close)
			transport := NewTransport(server.Client(), "", nil)
			err := transport.NegotiateAPIVersion(t.Context(), server.URL)
			protocol, ok := errors.AsType[*ProtocolError](err)
			if !ok || protocol.Retryable() || !strings.Contains(err.Error(), version) || !strings.Contains(err.Error(), "3.6") || !strings.Contains(protocol.CorrectiveAction(), "Upgrade") {
				t.Fatalf("unsupported server error = %T %v", err, err)
			}
			if transport.APIVersion() != "" || RequestID(err) != "discovery-unsupported" {
				t.Fatal("unsupported discovery selected a version or lost its request ID")
			}
		})
	}
}

func TestNegotiateAPIVersionRequiresDocumentedSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse>`)
	}))
	t.Cleanup(server.Close)
	transport := NewTransport(server.Client(), "", nil)
	err := transport.NegotiateAPIVersion(t.Context(), server.URL)
	protocol, ok := errors.AsType[*ProtocolError](err)
	if !ok || protocol.HTTPStatus() != http.StatusCreated || transport.APIVersion() != "" {
		t.Fatalf("unexpected discovery status = %T %v", err, err)
	}
}

func TestNegotiateAPIVersionBoundsResponseAndPreservesUpstreamAdvice(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"oversized", 200, strings.Repeat("x", 64*1024+1)},
		{"upstream", 503, `<tsResponse><error code="503000"><summary>Unavailable</summary><detail>Try later</detail></error></tsResponse>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-Tableau-Request-Id", "discovery-failed")
				w.Header().Set("Retry-After", "2")
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			t.Cleanup(server.Close)
			transport := NewTransport(server.Client(), "", nil)
			err := transport.NegotiateAPIVersion(t.Context(), server.URL)
			if err == nil || RequestID(err) != "discovery-failed" || transport.APIVersion() != "" {
				t.Fatalf("discovery failure = %v", err)
			}
			if test.name == "upstream" {
				upstream, ok := errors.AsType[*UpstreamError](err)
				if !ok || upstream.HTTPStatus() != 503 || !upstream.Retryable() || upstream.CorrectiveAction() == "" {
					t.Fatalf("upstream context = %T %v", err, err)
				}
				if delay, set := RetryAfter(err); !set || delay != 2*time.Second {
					t.Fatalf("Retry-After = %v, %v", delay, set)
				}
			} else if !strings.Contains(err.Error(), "exceeded") {
				t.Fatalf("response bound error = %v", err)
			}
		})
	}
}
