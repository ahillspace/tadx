package tableau

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// renewingSession sends "stale" until renewed, then "current".
type renewingSession struct {
	mu       sync.Mutex
	token    string
	rejected []string
	err      error
}

func newRenewingSession() *renewingSession { return &renewingSession{token: "stale"} }

func (s *renewingSession) Authorize(request *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	request.Header.Set("X-Tableau-Auth", s.token)
}
func (s *renewingSession) SiteLUID() string { return "site-luid" }
func (s *renewingSession) UserLUID() string { return "user-luid" }
func (s *renewingSession) String() string   { return "redacted session" }
func (s *renewingSession) Renew(_ context.Context, rejected http.Header) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejected = append(s.rejected, rejected.Get("X-Tableau-Auth"))
	if s.err != nil {
		return s.err
	}
	s.token = "current"
	return nil
}

type sentRequest struct {
	token       string
	length      int64
	contentType string
	body        []byte
}

// rejectingServer rejects the stale token with the given response and
// accepts any other token, recording every request it receives.
func rejectingServer(t *testing.T, status int, contentType, rejection string) (*httptest.Server, *[]sentRequest) {
	t.Helper()
	var mu sync.Mutex
	var sent []sentRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		mu.Lock()
		sent = append(sent, sentRequest{token: request.Header.Get("X-Tableau-Auth"), length: request.ContentLength, contentType: request.Header.Get("Content-Type"), body: body})
		mu.Unlock()
		if request.Header.Get("X-Tableau-Auth") == "stale" {
			writer.Header().Set("Content-Type", contentType)
			writer.WriteHeader(status)
			_, _ = io.WriteString(writer, rejection)
			return
		}
		_, _ = io.WriteString(writer, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)
	return server, &sent
}

const restSessionRejection = `<?xml version='1.0' encoding='UTF-8'?><tsResponse xmlns="http://tableau.com/api"><error code="401002"><summary>Unauthorized Access</summary><detail>Invalid authentication credentials were provided.</detail></error></tsResponse>`

func TestTransportRenewsARejectedSessionAndResendsTheSameRequestOnce(t *testing.T) {
	content := bytes.Repeat([]byte("w"), 32*1024)
	rejections := map[string]struct{ contentType, body string }{
		"rest code 401002": {"application/xml", restSessionRejection},
		"metadata":         {"application/json", `{"timestamp":1,"status":401,"error":"Unauthorized","path":"/relationship-service-war/graphql"}`},
		"search":           {"application/json", `{"errors":[{"errorCode":"AUTHENTICATION_ERROR"}]}`},
		"pulse":            {"application/json", `{"httpErrorCode":401,"message":"Unable to authenticate user"}`},
	}
	for name, rejection := range rejections {
		for _, body := range []string{"bytes", "multipart"} {
			t.Run(name+" "+body, func(t *testing.T) {
				server, sent := rejectingServer(t, http.StatusUnauthorized, rejection.contentType, rejection.body)
				request := Request{Method: http.MethodPost, ServerURL: server.URL, Path: "/api/3.29/sites/site-luid/projects", Operation: "project.create"}
				var expected []byte
				if body == "bytes" {
					request.Body, request.ContentType = []byte("<tsRequest/>"), "application/xml"
					expected = request.Body
				} else {
					multipart, err := NewMultipartBody(RequestPayloadPart([]byte("<tsRequest/>")), FilePart("tableau_workbook", "Finance.twbx", bytes.NewReader(content), 0, int64(len(content))))
					if err != nil {
						t.Fatal(err)
					}
					request.Multipart = multipart
					expected, _ = io.ReadAll(multipart.open())
				}
				session := newRenewingSession()
				response, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), session, request)
				if err != nil {
					t.Fatal(err)
				}
				if string(response.Body) != `{"ok":true}` || len(session.rejected) != 1 || session.rejected[0] != "stale" {
					t.Fatalf("response %q, renewals for %q", response.Body, session.rejected)
				}
				if len(*sent) != 2 || (*sent)[0].token != "stale" || (*sent)[1].token != "current" {
					t.Fatalf("sent %d requests: %+v", len(*sent), *sent)
				}
				first, second := (*sent)[0], (*sent)[1]
				if !bytes.Equal(first.body, expected) || !bytes.Equal(second.body, expected) || first.length != second.length || first.contentType != second.contentType {
					t.Fatal("the resent request differs from the rejected request")
				}
			})
		}
	}
}

func TestTransportRenewsOnlySessionRejections(t *testing.T) {
	for name, rejection := range map[string]struct {
		status int
		body   string
	}{
		"sign-in error 401001": {http.StatusUnauthorized, `<tsResponse><error code="401001"><summary>Signin Error</summary></error></tsResponse>`},
		"forbidden":            {http.StatusForbidden, `<tsResponse><error code="403004"><summary>Forbidden</summary></error></tsResponse>`},
		"server error":         {http.StatusInternalServerError, `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			server, sent := rejectingServer(t, rejection.status, "application/xml", rejection.body)
			session := newRenewingSession()
			_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), session, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/api/3.29/sites", Operation: "site.list"})
			if upstream, ok := errors.AsType[*UpstreamError](err); !ok || upstream.StatusCode != rejection.status {
				t.Fatalf("error = %v", err)
			}
			if len(session.rejected) != 0 || len(*sent) != 1 {
				t.Fatalf("renewals %q, requests %d; want neither renewal nor resend", session.rejected, len(*sent))
			}
		})
	}
}

func TestTransportReturnsASecondRejectionWithoutRenewingAgain(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/xml")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, restSessionRejection)
	}))
	defer server.Close()
	session := newRenewingSession()
	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), session, Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/api/3.29/sites", Operation: "site.list"})
	if upstream, ok := errors.AsType[*UpstreamError](err); !ok || upstream.Code != "401002" {
		t.Fatalf("error = %v", err)
	}
	if len(session.rejected) != 1 || requests != 2 {
		t.Fatalf("renewals %q, requests %d; want one renewal and one resend", session.rejected, requests)
	}
}

func TestTransportReportsTheRejectionFirstWhenRenewalFails(t *testing.T) {
	server, sent := rejectingServer(t, http.StatusUnauthorized, "application/xml", restSessionRejection)
	session := newRenewingSession()
	session.err = errors.New("sign in: PAT was revoked")
	_, err := NewTransport(server.Client(), "3.29", nil).Do(t.Context(), session, Request{Method: http.MethodPost, ServerURL: server.URL, Path: "/api/3.29/sites/site-luid/projects", Body: []byte("<tsRequest/>"), Operation: "project.create"})
	upstream, ok := errors.AsType[*UpstreamError](err)
	if !ok || upstream.Code != "401002" || !errors.Is(err, session.err) || !SubmissionAttempted(err) {
		t.Fatalf("error = %v; want the rejection, then the renewal failure", err)
	}
	if !strings.HasPrefix(err.Error(), upstream.Error()) || !strings.Contains(err.Error(), "signing in again failed") {
		t.Fatalf("error message = %q", err)
	}
	if len(*sent) != 1 {
		t.Fatalf("requests = %d, want no resend after a failed renewal", len(*sent))
	}
}
