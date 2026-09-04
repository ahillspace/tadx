package pulse_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	pulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestDeleteUsesOneExactRequestAndPreservesOutcome(t *testing.T) {
	for _, kind := range []string{"definition", "metric"} {
		for _, status := range []int{200, 202, 204, 400, 403, 404, 409, 500} {
			t.Run(fmt.Sprintf("%s/%d", kind, status), func(t *testing.T) {
				calls := 0
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					body, _ := io.ReadAll(r.Body)
					if r.Method != "DELETE" || r.URL.EscapedPath() != "/api/-/pulse/"+kind+"s/exact%2Fid" || r.URL.RawQuery != "" || len(body) != 0 {
						t.Errorf("request=%s %s body=%s", r.Method, r.URL, body)
					}
					if r.Header.Get("X-Tableau-Auth") != "token" {
						t.Error("missing session authentication")
					}
					w.Header().Set("X-Tableau-Request-Id", "delete-request")
					w.WriteHeader(status)
					if status >= 400 {
						_, _ = io.WriteString(w, `{"error":{"code":"dependency-code","summary":"Deletion rejected","detail":"Tableau dependency detail"}}`)
					}
				}))
				defer server.Close()
				client := pulse.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
				var result pulse.DeleteResult
				var err error
				if kind == "definition" {
					result, err = client.DeleteDefinition(context.Background(), "exact/id")
				} else {
					result, err = client.DeleteMetric(context.Background(), "exact/id")
				}
				if calls != 1 {
					t.Fatalf("calls=%d", calls)
				}
				if status >= 400 {
					var upstream *tableau.UpstreamError
					if !errors.As(err, &upstream) || upstream.StatusCode != status || upstream.Code != "dependency-code" || upstream.Detail != "Tableau dependency detail" || upstream.TableauRequestID != "delete-request" {
						t.Fatalf("err=%#v", err)
					}
					return
				}
				if err != nil || result.LUID != "exact/id" || result.HTTPStatus != status || result.TableauRequestID != "delete-request" {
					t.Fatalf("result=%#v err=%v", result, err)
				}
				if status == 202 && result.Status != "accepted" {
					t.Fatalf("asynchronous result=%#v", result)
				}
			})
		}
	}
}

func TestDeleteRejectsEmptyIdentityBeforeRequest(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }))
	defer server.Close()
	client := pulse.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if _, err := client.DeleteDefinition(context.Background(), " "); err == nil {
		t.Error("accepted empty definition ID")
	}
	if _, err := client.DeleteMetric(context.Background(), " "); err == nil {
		t.Error("accepted empty metric ID")
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}
