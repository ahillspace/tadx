package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIndependentClientAuthenticatesAndProtectsCredentials(t *testing.T) {
	t.Setenv("LIVE_FIXTURE_NAME", "fixture-name")
	t.Setenv("LIVE_FIXTURE_SECRET", "fixture-secret")
	var observed []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = append(observed, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/2.4/serverinfo":
			fmt.Fprint(w, `{"serverInfo":{"restApiVersion":"3.29"}}`)
		case "/api/3.29/auth/signin":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			credentials, _ := body["credentials"].(map[string]any)
			if credentials["personalAccessTokenName"] != "fixture-name" || credentials["personalAccessTokenSecret"] != "fixture-secret" {
				t.Error("sign-in lost independent PAT")
			}
			fmt.Fprint(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-test"},"user":{"id":"user-test"}}}`)
		case "/api/3.29/auth/signout":
			if r.Header.Get("X-Tableau-Auth") != "fixture-session" {
				t.Error("signout omitted session")
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"fixture-secret fixture-session"}`)
		}
	}))
	defer server.Close()
	client, err := NewClient(t.Context(), Profile{ServerURL: server.URL, ExpectedSiteLUID: "site-test", FixturePAT: Credential{NameEnv: "LIVE_FIXTURE_NAME", SecretEnv: "LIVE_FIXTURE_SECRET"}})
	if err != nil {
		t.Fatal(err)
	}
	if client.SiteID != "site-test" || client.UserID != "user-test" || client.Version != "3.29" {
		t.Fatal("authoritative session identity lost")
	}
	_, err = client.JSON(t.Context(), http.MethodGet, "/forbidden", nil)
	if err == nil || strings.Contains(err.Error(), "fixture-secret") || strings.Contains(err.Error(), "fixture-session") {
		t.Fatal("HTTP error exposed upstream body or failed to reject response")
	}
	if client.Redact("fixture-name fixture-secret fixture-session") != "[REDACTED] [REDACTED] [REDACTED]" {
		t.Fatal("credential redaction incomplete")
	}
	if formatted := fmt.Sprintf("%v %#v", client, client); strings.Contains(formatted, "fixture-") {
		t.Fatal("client diagnostic formatting exposed credentials")
	}
	if err := client.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 4 {
		t.Fatalf("unexpected independent requests: %v", observed)
	}
}

func TestIndependentListReadsAllPagesAndPreservesQuery(t *testing.T) {
	var pages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("pageNumber"))
		if r.URL.Query().Get("filter") != "name:eq:fixture" {
			t.Error("list lost filter")
		}
		switch r.URL.Query().Get("pageNumber") {
		case "1":
			fmt.Fprint(w, `{"pagination":{"totalAvailable":"2"},"projects":{"project":[{"id":"first","name":"fixture","parentProjectId":"parent"}]}}`)
		case "2":
			fmt.Fprint(w, `{"pagination":{"totalAvailable":"2"},"projects":{"project":[{"id":"second","name":"fixture","parentProjectId":"parent"}]}}`)
		default:
			t.Error("unexpected page")
		}
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	query := url.Values{"pageSize": {"1"}, "filter": {"name:eq:fixture"}}
	objects, err := client.List(t.Context(), "project", query)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 2 || objects[1].ID != "second" || objects[1].ProjectID != "parent" {
		t.Fatalf("pagination identity mismatch: %#v", objects)
	}
	if len(pages) != 2 || query.Get("pageNumber") != "" {
		t.Fatal("pagination mutated input or skipped a page")
	}
}

func TestIndependentTransportRejectsRedirectAndAbsoluteURL(t *testing.T) {
	var leaked bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked = true; w.WriteHeader(200) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, http.StatusFound) }))
	defer server.Close()
	client := &Client{Server: server.URL, HTTP: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, token: "test-session"}
	if _, err := client.Do(t.Context(), "GET", destination.URL, nil, ""); err == nil {
		t.Fatal("absolute request accepted")
	}
	if _, err := client.Do(t.Context(), "GET", "/redirect", nil, ""); err == nil {
		t.Fatal("redirect accepted")
	}
	if leaked {
		t.Fatal("session request followed cross-origin redirect")
	}
}

func TestIndependentMetadataRejectsPartialGraphQLError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/graphql" || r.Method != "POST" {
			t.Error("wrong metadata request")
		}
		fmt.Fprint(w, `{"data":{"tables":[]},"errors":[{"message":"private provider details"}]}`)
	}))
	defer server.Close()
	client := &Client{Server: server.URL, HTTP: server.Client()}
	if _, err := client.Metadata(t.Context(), "{tables{id}}", nil); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("partial GraphQL observation accepted or leaked")
	}
}

func TestIndependentPublishUsesNativeMultipartAndExactAsyncIdentity(t *testing.T) {
	filename := writeNativeTestFile(t, "fixture.tds", []byte(`<datasource name="source"><column name="[Sales]"/></datasource>`))
	var observedJob string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST":
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/mixed;") {
				t.Error("publish omitted mixed MIME content type")
			}
			reader, err := r.MultipartReader()
			if err != nil {
				t.Error(err)
				return
			}
			request, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			body, _ := io.ReadAll(request)
			if request.Header.Get("Content-Disposition") != `name="request_payload"` || !strings.Contains(string(body), `name="owned"`) || !strings.Contains(string(body), `id="project-id"`) {
				t.Errorf("wrong XML publish request: %s", body)
			}
			native, err := reader.NextPart()
			if err != nil {
				t.Error(err)
				return
			}
			if native.Header.Get("Content-Disposition") != `name="tableau_datasource"; filename="fixture.tds"` {
				t.Error("wrong native multipart part")
			}
			fmt.Fprint(w, `{"job":{"id":"exact-job"}}`)
		case r.Method == "GET":
			observedJob = r.URL.Path
			fmt.Fprint(w, `{"job":{"id":"exact-job","progress":"100","finishCode":"0","datasource":{"id":"published-id"}}}`)
		}
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	object, err := client.Publish(t.Context(), "datasource", filename, "project-id", "owned", false)
	if err != nil {
		t.Fatal(err)
	}
	if object.ID != "published-id" || object.Name != "" || object.ProjectID != "" || observedJob != "/api/3.29/sites/site/jobs/exact-job" {
		t.Fatal("publish did not reconcile exact async resource")
	}
}

func TestIndependentJobRejectsDifferentIdentityAndRecognizes404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"job":{"id":"other-job","finishCode":"0"}}`)
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	if _, err := client.WaitJob(t.Context(), "requested-job"); err == nil {
		t.Fatal("unrelated job accepted")
	}
	if !IsNotFound(fmt.Errorf("wrapped: %w", &HTTPError{Status: 404})) || IsNotFound(errors.New("404")) {
		t.Fatal("not-found classification is not authoritative")
	}
}

func TestIndependentJobDoesNotTreatActiveFinishCodeAsTerminal(t *testing.T) {
	for name, body := range map[string]string{
		"active-extract":  `{"job":{"id":"exact","type":"RefreshExtract","progress":"25","finishCode":"0"}}`,
		"pending-publish": `{"job":{"id":"exact","type":"PublishDatasource","progress":"0","finishCode":"1"}}`,
		"bridge-assigned": `{"job":{"id":"exact","type":"BridgeRefreshExtract","progress":"100","finishCode":"0"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
			ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
			defer cancel()
			_, err := client.WaitJob(ctx, "exact")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("active job treated as terminal: %v", err)
			}
		})
	}
}

func TestIndependentWritePolicyRunsBeforeNetworkAndReadPostsRemainReads(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; fmt.Fprint(w, `{}`) }))
	defer server.Close()
	checks := 0
	denied := errors.New("test policy denied")
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(), AuthorizeWrite: func(context.Context, string, string) error { checks++; return denied }}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		if _, err := client.Do(t.Context(), method, "/api/3.29/sites/site/groups", nil, ""); !errors.Is(err, denied) {
			t.Fatalf("%s policy denial lost: %v", method, err)
		}
	}
	if requests != 0 || checks != 4 {
		t.Fatal("write reached network before fresh policy authorization")
	}
	for _, path := range []string{"/api/metadata/graphql", "/api/-/pulse/definitions:batchGet?view=DEFINITION_VIEW_BASIC", "/api/-/pulse/metrics:batchGet", "/api/3.29/auth/signout"} {
		if _, err := client.Do(t.Context(), "POST", path, nil, ""); err != nil {
			t.Fatal(err)
		}
	}
	if checks != 4 || requests != 4 {
		t.Fatal("read or authentication POST incorrectly treated as consequential mutation")
	}
}

func TestSerialVerifierGuardBlocksRefreshBeforeAnyNetwork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	denied := Inconclusive("detached worker is alive")
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", UserID: "user", HTTP: server.Client(), patName: "private-name", patSecret: "private-secret", token: "stale-token", BeforeRefresh: func(context.Context) error { return denied }}
	client.MarkSessionStale()
	if _, err := client.Do(t.Context(), "GET", client.REST("projects"), nil, ""); !errors.Is(err, denied) {
		t.Fatalf("worker guard denial lost: %v", err)
	}
	if requests != 0 || !client.SessionStale {
		t.Fatal("blocked refresh changed session or reached network")
	}
	client.BeforeRefresh = nil
	if _, err := client.Do(t.Context(), "GET", client.REST("projects"), nil, ""); ErrorStatus(err) != InconclusiveStatus {
		t.Fatalf("unguarded serial refresh accepted: %v", err)
	}
	if requests != 0 {
		t.Fatal("unguarded refresh reached network")
	}
}

func TestSerialVerifierRefreshesSamePATBeforeObservation(t *testing.T) {
	var order []string
	guarded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			if !guarded {
				t.Error("signin preceded worker guard")
			}
			if r.Header.Get("X-Tableau-Auth") != "" {
				t.Error("stale session was sent with PAT signin")
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			credentials, _ := request["credentials"].(map[string]any)
			site, _ := credentials["site"].(map[string]any)
			if credentials["personalAccessTokenName"] != "private-name" || credentials["personalAccessTokenSecret"] != "private-secret" || site["contentUrl"] != "" {
				t.Error("refresh changed private saved PAT or explicit default site")
			}
			fmt.Fprint(w, `{"credentials":{"token":"fresh-session","site":{"id":"site"},"user":{"id":"user"}}}`)
		case "/api/3.29/sites/site/projects":
			if r.Header.Get("X-Tableau-Auth") != "fresh-session" {
				t.Error("observation did not use refreshed verifier session")
			}
			fmt.Fprint(w, `{"projects":{"project":[]}}`)
		default:
			t.Errorf("unexpected refresh path %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", UserID: "user", HTTP: server.Client(), patName: "private-name", patSecret: "private-secret", token: "stale-session", BeforeRefresh: func(context.Context) error { guarded = true; return nil }}
	client.MarkSessionStale()
	if _, err := client.JSON(t.Context(), "GET", client.REST("projects"), nil); err != nil {
		t.Fatal(err)
	}
	if client.SessionStale || client.SiteID != "site" || client.UserID != "user" || len(order) != 2 || order[0] != "POST /api/3.29/auth/signin" || order[1] != "GET /api/3.29/sites/site/projects" {
		t.Fatalf("serial refresh did not preserve exact observation identity/order: %v", order)
	}
}

func TestSerialVerifierRejectsRefreshWithDifferentIdentity(t *testing.T) {
	for _, changed := range []string{"site", "user"} {
		t.Run(changed, func(t *testing.T) {
			requests := 0
			observations := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/api/3.29/auth/signin" {
					observations++
					w.WriteHeader(500)
					return
				}
				site, user := "site", "user"
				if changed == "site" {
					site = "other-site"
				} else {
					user = "other-user"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"credentials": map[string]any{"token": "new-session", "site": map[string]string{"id": site}, "user": map[string]string{"id": user}}})
			}))
			defer server.Close()
			client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", UserID: "user", HTTP: server.Client(), patName: "private-name", patSecret: "private-secret", token: "old-session", BeforeRefresh: func(context.Context) error { return nil }}
			client.MarkSessionStale()
			if _, err := client.Do(t.Context(), "GET", client.REST("projects"), nil, ""); err == nil {
				t.Fatal("changed authoritative refresh identity accepted")
			}
			if requests != 1 || observations != 0 || client.SiteID != "site" || client.UserID != "user" || client.token != "old-session" || !client.SessionStale {
				t.Fatal("failed refresh changed trusted identity or observed another target")
			}
		})
	}
}

func TestSerialVerifierNeverRetriesFailedMutationOrUnauthorizedResponse(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			signins, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/3.29/auth/signin" {
					signins++
					fmt.Fprint(w, `{"credentials":{"token":"fresh-session","site":{"id":"site"},"user":{"id":"user"}}}`)
					return
				}
				writes++
				w.WriteHeader(status)
			}))
			defer server.Close()
			client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", UserID: "user", HTTP: server.Client(), patName: "private-name", patSecret: "private-secret", token: "old-session", BeforeRefresh: func(context.Context) error { return nil }}
			client.MarkSessionStale()
			if _, err := client.JSON(t.Context(), "POST", client.REST("groups"), map[string]any{"group": map[string]string{"name": "owned"}}); err == nil {
				t.Fatal("failed mutation returned success")
			}
			if signins != 1 || writes != 1 {
				t.Fatalf("failed mutation retried or authenticated after outcome: signins %d, writes %d", signins, writes)
			}
		})
	}
}

func TestStaleVerifierSignoutSkipsRefreshAndMutationHooks(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/3.29/auth/signout" || r.Header.Get("X-Tableau-Auth") != "old-session" {
			t.Error("unexpected auth request")
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", HTTP: server.Client(), token: "old-session", SessionStale: true, BeforeRefresh: func(context.Context) error { t.Error("signout called worker guard"); return errors.New("unexpected") }, AuthorizeWrite: func(context.Context, string, string) error {
		t.Error("signout called mutation policy")
		return errors.New("unexpected")
	}}
	if err := client.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatal("signout refreshed or repeated authentication")
	}
}

func TestProjectExactReadUsesCompletePaginatedInventory(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path+"?"+r.URL.RawQuery)
		if r.URL.Path != "/api/3.29/sites/site/projects" {
			t.Errorf("unsupported exact project endpoint requested: %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		switch r.URL.Query().Get("pageNumber") {
		case "1":
			fmt.Fprint(w, `{"pagination":{"totalAvailable":"2"},"projects":{"project":[{"id":"decoy","name":"same-name"}]}}`)
		case "2":
			fmt.Fprint(w, `{"pagination":{"totalAvailable":"2"},"projects":{"project":[{"id":"wanted","name":"same-name","parentProjectId":"root"}]}}`)
		default:
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	project, err := client.Get(t.Context(), "project", "wanted")
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "wanted" || project.ProjectID != "root" || len(requests) != 2 {
		t.Fatalf("exact project lookup did not traverse complete inventory: %#v %v", project, requests)
	}
	requests = nil
	if _, err := client.Get(t.Context(), "project", "absent"); !IsNotFound(err) {
		t.Fatalf("complete inventory did not confirm exact absence: %v", err)
	}
	if len(requests) != 2 {
		t.Fatal("project absence asserted before all inventory pages")
	}
}

func TestProjectLookupCannotAssertAbsenceFromIncompleteInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"projects":{"project":[]}}`) }))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	if _, err := client.Get(t.Context(), "project", "absent"); err == nil || IsNotFound(err) {
		t.Fatalf("incomplete project inventory claimed exact absence: %v", err)
	}
}

func TestPublishWithoutResourceInTerminalJobReconcilesOnlyCompleteExactInventory(t *testing.T) {
	for _, mode := range []string{"delayed-exact", "missing", "ambiguous", "incomplete", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			filename := writeNativeTestFile(t, "fixture.tds", []byte(`<datasource><column name="[Sales]"/></datasource>`))
			writes, reads := 0, 0
			journal := testJournal(t)
			caseState := &Case{Journal: journal}
			wanted := "tadx-live-safety-published"
			if err := caseState.Intent("datasource", wanted, "project-one"); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "POST":
					writes++
					fmt.Fprint(w, `{"job":{"id":"publish-job-only"}}`)
				case r.URL.Path == "/api/3.29/sites/site-one/jobs/publish-job-only":
					fmt.Fprint(w, `{"job":{"id":"publish-job-only","type":"PublishDatasource","progress":"100","finishCode":"0","mode":"Asynchronous","completedAt":"2026-01-01T00:00:00Z","statusNotes":{}}}`)
				case r.URL.Path == "/api/3.29/sites/site-one/datasources":
					reads++
					row := func(id, name, parent string) map[string]any {
						return map[string]any{"id": id, "name": name, "project": map[string]any{"id": parent}}
					}
					var rows []any
					total := 0
					switch mode {
					case "delayed-exact":
						if reads == 1 {
							break
						}
						total = 2
						if r.URL.Query().Get("pageNumber") == "1" {
							rows = []any{row("same-name-decoy", wanted, "other-project")}
						} else {
							rows = []any{row("real-resource", wanted, "project-one")}
						}
					case "ambiguous":
						rows = []any{row("first", wanted, "project-one"), row("second", wanted, "project-one")}
						total = 2
					case "duplicate":
						rows = []any{row("repeated", wanted, "project-one"), row("repeated", wanted, "project-one")}
						total = 2
					case "incomplete":
						fmt.Fprint(w, `{"datasources":{"datasource":[]}}`)
						return
					}
					if rows == nil {
						rows = []any{}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"datasources": map[string]any{"datasource": rows}, "pagination": map[string]any{"totalAvailable": total}})
				default:
					t.Errorf("unexpected reconciliation request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			api := &Client{Server: server.URL, Version: "3.29", SiteID: "site-one", HTTP: server.Client(), JournalJob: caseState.AcceptJob, publishObservationBudget: time.Second}
			if mode == "missing" {
				api.publishObservationBudget = 40 * time.Millisecond
			}
			object, err := api.Publish(t.Context(), "datasource", filename, "project-one", wanted, false)
			if mode == "delayed-exact" {
				if err != nil {
					t.Fatal(err)
				}
				if object.ID != "real-resource" || object.Name != wanted || object.ProjectID != "project-one" || reads != 3 {
					t.Fatalf("publish selected wrong identity or skipped complete inventory: %#v reads=%d", object, reads)
				}
				if err := caseState.ConfirmReceipt("datasource", wanted, "project-one", object); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || object.ID != "" {
					t.Fatalf("unresolved publish fabricated resource identity from job: %#v %v", object, err)
				}
			}
			if writes != 1 {
				t.Fatalf("read reconciliation repeated publish mutation %d times", writes)
			}
			saved, loadErr := LoadJournal(journal.path, journal.Target)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if len(saved.Jobs) != 1 || saved.Jobs[0] != "publish-job-only" {
				t.Fatal("accepted publish job identity was not durable")
			}
			if mode != "delayed-exact" && (saved.Entries[0].Object != nil || saved.Entries[0].Done) {
				t.Fatal("unknown published identity was replaced with job identity or discarded pending intent")
			}
		})
	}
}

func TestIndependentMutationsRejectOtherTargetsBeforeAuthorizationOrNetwork(t *testing.T) {
	requests, authorizations := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(), AuthorizeWrite: func(context.Context, string, string) error { authorizations++; return nil }}
	for _, path := range []string{
		"/submit", "/api/3.29/schedules", "/api/3.29/sites/other-site/groups", "/api/3.29/sites/site-other/groups",
		"/api/3.29/sites/site/groups/../projects", "/api/3.29/sites/site/groups/%2e%2e/projects", "/api/3.29/sites/site/groups/%252e%252e/projects",
		"/api/3.29/sites/site/groups/id%2fother", "/api/3.29/sites/site/groups/id%5cother", "/api/3.29/sites/site/groups/%00id",
		"/api/3.29/sites/site//groups", "/api/3.29/sites/site/groups/", "/api/3.29/sites/site/groups#fragment",
		"/api/-/pulse/../definitions", "/api/3.29/sites/other-site/auth/signin", "/api/3.28/auth/signin",
	} {
		t.Run(path, func(t *testing.T) {
			if _, err := client.Do(t.Context(), http.MethodPost, path, []byte(`{}`), "application/json"); err == nil {
				t.Fatal("mutation escaped selected site boundary")
			}
		})
	}
	if requests != 0 || authorizations != 0 {
		t.Fatalf("invalid target reached authorization/network: %d/%d", authorizations, requests)
	}
	client.SiteID = ""
	if _, err := client.Do(t.Context(), http.MethodPost, "/api/-/pulse/definitions", []byte(`{}`), "application/json"); err == nil {
		t.Fatal("Pulse mutation without verified site identity accepted")
	}
	if requests != 0 || authorizations != 0 {
		t.Fatal("missing-site mutation reached authorization or network")
	}
}

func TestIndependentMutationBoundaryAllowsNativeQueryAndSelectedPulseHeader(t *testing.T) {
	requests, authorizations := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/3.29/sites/site/datasources":
			if r.URL.Query().Get("asJob") != "true" || r.URL.Query().Get("overwrite") != "false" || !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/mixed;") {
				t.Error("target validation changed native publish query or MIME body")
			}
		case "/api/-/pulse/definitions":
			if r.Header.Get("X-Tableau-Site-Id") != "site" {
				t.Error("Pulse mutation omitted selected authenticated site header")
			}
		default:
			t.Errorf("unexpected accepted mutation %s", r.URL.Path)
		}
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client := &Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client(), AuthorizeWrite: func(context.Context, string, string) error { authorizations++; return nil }}
	body := []byte("--fixture\r\nContent-Disposition: name=\"request_payload\"\r\n\r\n<tsRequest/>\r\n--fixture--\r\n")
	if _, err := client.Do(t.Context(), http.MethodPost, client.REST("datasources")+"?asJob=true&overwrite=false&datasourceType=tdsx", body, "multipart/mixed; boundary=fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.JSON(t.Context(), http.MethodPost, "/api/-/pulse/definitions", map[string]any{"name": "owned"}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || authorizations != 2 {
		t.Fatalf("valid scoped mutations skipped policy/network: %d/%d", authorizations, requests)
	}
}

// Compile-time proof that observation methods accept caller cancellation.
var _ func(context.Context, string) (Object, error) = (&Client{}).WaitJob
