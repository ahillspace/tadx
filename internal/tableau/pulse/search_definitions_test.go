package pulse_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestSearchDefinitionsSendsNameFilterAndReadsOneProviderPage(t *testing.T) {
	for _, test := range []struct {
		name, term, filter string
	}{
		{name: "trimmed", term: "  test  ", filter: "name:has:test"},
		{name: "space", term: "sales ops", filter: "name:has:sales%20ops"},
		{name: "reserved and Unicode", term: "Café + A/B?x=1&y=two,three:*(!')~%", filter: "name:has:Caf%C3%A9%20%2B%20A%2FB%3Fx%3D1%26y%3Dtwo%2Cthree%3A*(!')~%25"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls++
				query := request.URL.Query()
				if request.Method != http.MethodGet || request.URL.Path != "/api/-/pulse/definitions" || query.Get("filter") != test.filter || query.Get("page_size") != "2" || query.Get("page_token") != "token-1" || len(query) != 3 {
					t.Errorf("request=%s %s", request.Method, request.URL)
				}
				if request.Header.Get("X-Tableau-Auth") != "token" || request.Header.Get("X-Tableau-Site-Id") != "site-1" {
					t.Error("missing authenticated site headers")
				}
				writer.Header().Set("X-Tableau-Request-Id", "search-request")
				_, _ = io.WriteString(writer, `{"definitions":[{"metadata":{"id":"definition-1","name":"Server-selected result"},"specification":{"datasource":{"id":"datasource-1"}}}],"next_page_token":"token-2"}`)
			}))
			defer server.Close()
			client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
			page, err := client.SearchDefinitions(t.Context(), test.term, tableaupulse.PageRequest{PageSize: 2, PageToken: "token-1"})
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(page.Definitions) != 1 || page.Definitions[0].LUID != "definition-1" || page.Definitions[0].Name != "Server-selected result" || page.NextPageToken != "token-2" || page.TableauRequestID != "search-request" {
				t.Fatalf("calls=%d page=%+v", calls, page)
			}
		})
	}
}

func TestSearchDefinitionsRejectsBlankTermAndInvalidPageBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { calls++ }))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	if _, err := client.SearchDefinitions(t.Context(), " \t ", tableaupulse.PageRequest{PageSize: 20}); err == nil {
		t.Fatal("accepted blank search term")
	}
	if _, err := client.SearchDefinitions(t.Context(), "test", tableaupulse.PageRequest{PageSize: 101}); err == nil {
		t.Fatal("accepted oversized page")
	}
	if calls != 0 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestSearchDefinitionsPreservesRateLimitAdviceWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls++
		writer.Header().Set("X-Tableau-Request-Id", "rate-limit-request")
		writer.Header().Set("Retry-After", "28")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(writer, `{"httpErrorCode":429,"message":"Too Many Requests"}`)
	}))
	defer server.Close()
	client := newPulseClient(t, tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	_, err := client.SearchDefinitions(t.Context(), "test", tableaupulse.PageRequest{PageSize: 20})
	upstream, ok := errors.AsType[*tableau.UpstreamError](err)
	if !ok {
		t.Fatalf("err=%v", err)
	}
	delay, present := upstream.RetryAfter()
	if calls != 1 || upstream.HTTPStatus() != 429 || !upstream.Retryable() || !present || delay != 28*time.Second || upstream.RequestID() != "rate-limit-request" {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
