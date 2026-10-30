package tableau

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

type countingWriter struct{ written int64 }

func (w *countingWriter) Write(data []byte) (int, error) {
	w.written += int64(len(data))
	return len(data), nil
}

func streamTo(destination io.Writer) func(Response) (io.Writer, error) {
	return func(Response) (io.Writer, error) { return destination, nil }
}

func TestTransportStreamedRequestRejectedFromHeadersWritesNothing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Disposition", `attachment; filename="../escape.tdsx"`)
		_, _ = io.WriteString(writer, "package bytes")
	}))
	defer server.Close()

	rejected := errors.New("unsafe filename")
	var destination bytes.Buffer
	var seen string
	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "datasource.pull",
		OpenResponseBody: func(response Response) (io.Writer, error) {
			seen = response.Header.Get("Content-Disposition")
			return &destination, rejected
		},
	})
	if !errors.Is(err, rejected) || seen != `attachment; filename="../escape.tdsx"` {
		t.Fatalf("error = %v, header seen = %q", err, seen)
	}
	if destination.Len() != 0 {
		t.Fatalf("a rejected download wrote %q", destination.String())
	}
}

func TestTransportStreamedRequestBoundsTheErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write(make([]byte, maxStreamedErrorBytes+1))
	}))
	defer server.Close()

	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "datasource.pull",
		MaxResponseBytes: defaultMaxResponseBytes, OpenResponseBody: func(Response) (io.Writer, error) {
			t.Error("an error response opened the destination")
			return io.Discard, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "body exceeded 4194304-byte limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestTransportStreamsASuccessfulBodyLargerThanTheBufferedLimitWithoutHoldingIt(t *testing.T) {
	const size = defaultMaxResponseBytes + 32*1024*1024
	chunk := make([]byte, 1024*1024)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("X-Tableau-Request-Id", "download-request")
		for sent := int64(0); sent < size; sent += int64(len(chunk)) {
			if _, err := writer.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	destination := &countingWriter{}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	response, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "datasource.pull", OpenResponseBody: streamTo(destination),
	})
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if destination.written != size {
		t.Fatalf("streamed %d bytes, want %d", destination.written, size)
	}
	if response.Body != nil || response.StatusCode != http.StatusOK || response.TableauRequestID != "download-request" || response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("response = status %d, request %q, header %v, body %d bytes", response.StatusCode, response.TableauRequestID, response.Header, len(response.Body))
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 32*1024*1024 {
		t.Fatalf("streaming %d bytes allocated %d bytes", size, allocated)
	}
}

func TestTransportStreamedRequestReturnsUpstreamErrorsWithoutWriting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		writer.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(writer, `<tsResponse xmlns="http://tableau.com/api"><error code="404004"><summary>Not Found</summary><detail>Datasource not found.</detail></error></tsResponse>`)
	}))
	defer server.Close()

	var destination bytes.Buffer
	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "datasource.pull", OpenResponseBody: streamTo(&destination),
	})
	upstream, ok := errors.AsType[*UpstreamError](err)
	if !ok || upstream.StatusCode != http.StatusNotFound || upstream.Code != "404004" {
		t.Fatalf("error = %#v", err)
	}
	if destination.Len() != 0 {
		t.Fatalf("an error response wrote %q", destination.String())
	}
}

func TestTransportStreamedRequestRenewsARejectedSessionAndWritesOneBody(t *testing.T) {
	server, sent := rejectingServer(t, http.StatusUnauthorized, "application/xml", restSessionRejection)
	session := newRenewingSession()
	var destination bytes.Buffer
	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), session, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/api/3.29/sites/site-luid/datasources/ds/content", Operation: "datasource.pull", OpenResponseBody: streamTo(&destination),
	})
	if err != nil {
		t.Fatal(err)
	}
	if destination.String() != `{"ok":true}` || len(*sent) != 2 || len(session.rejected) != 1 {
		t.Fatalf("wrote %q after %d requests and %d renewals", destination.String(), len(*sent), len(session.rejected))
	}
}

func TestTransportStreamedRequestReportsAStalledBody(t *testing.T) {
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
	var destination bytes.Buffer
	_, err := transport.Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "workbook.pull", OpenResponseBody: streamTo(&destination),
	})
	if err == nil || !strings.Contains(err.Error(), "read Tableau workbook.pull response: no response body progress for 50ms") {
		t.Fatalf("error = %v", err)
	}
	var advice interface{ Retryable() bool }
	if !errors.As(err, &advice) || !advice.Retryable() {
		t.Fatalf("a stalled read should be retryable: %#v", advice)
	}
}

func TestTransportStreamedRequestReportsADisconnectedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Length", "1048576")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("partial"))
		writer.(http.Flusher).Flush()
		connection, _, err := http.NewResponseController(writer).Hijack()
		if err == nil {
			_ = connection.(*net.TCPConn).SetLinger(0)
			_ = connection.Close()
		}
	}))
	defer server.Close()

	var destination bytes.Buffer
	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "flow.pull", OpenResponseBody: streamTo(&destination),
	})
	if _, ok := errors.AsType[*responseReadError](err); !ok || !strings.Contains(err.Error(), "read Tableau flow.pull response") {
		t.Fatalf("error = %v", err)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

func TestTransportStreamedRequestReportsADestinationFailureAsNotRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, "package bytes")
	}))
	defer server.Close()

	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), nil, Request{
		Method: http.MethodGet, ServerURL: server.URL, Path: "/content", Operation: "datasource.pull", OpenResponseBody: streamTo(failingWriter{}),
	})
	if err == nil || err.Error() != "write Tableau datasource.pull response: no space left on device" {
		t.Fatalf("error = %v", err)
	}
	var advice interface{ Retryable() bool }
	if errors.As(err, &advice) && advice.Retryable() {
		t.Fatal("a local write failure is not retryable")
	}
}
