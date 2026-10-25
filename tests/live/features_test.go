package live

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestTargetFeatureChecksUseOnlySelectedPrerequisites(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Errorf("unrelated feature probe %s", r.URL.Path)
		w.WriteHeader(500)
	}))
	defer server.Close()
	api := &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	if err := VerifyTargetFeatures(t.Context(), api, harness.Profile{Features: map[string]string{"pulse": "enabled", "metadata": "enabled"}}, []Spec{{ID: "live.project.create", Requires: []string{"Disposable parent; selected permission mode supported by target."}}}); err != nil {
		t.Fatal(err)
	}
	if requests != 0 {
		t.Fatal("unselected service prerequisites were probed")
	}
}

func TestCatalogVersionPrerequisitesRejectOlderTargetsBeforeAnyRequest(t *testing.T) {
	catalog, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ id, old, minimum string }{{"live.datasource.publish-replace", "3.24", "3.25"}, {"live.datasource.publish-composed", "3.28", "3.29"}} {
		t.Run(entry.id, func(t *testing.T) {
			var selected Spec
			for _, spec := range catalog {
				if spec.ID == entry.id {
					selected = spec
				}
			}
			if selected.ID == "" {
				t.Fatal("catalog entry missing")
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
			defer server.Close()
			api := &harness.Client{Server: server.URL, Version: entry.old, HTTP: server.Client()}
			if err := VerifyTargetFeatures(t.Context(), api, harness.Profile{}, []Spec{selected}); harness.ErrorStatus(err) != harness.UnsupportedStatus {
				t.Fatalf("known unsupported version was not typed unsupported: %v", err)
			}
			if requests != 0 {
				t.Fatal("known version failure still probed or mutated remote target")
			}
			api.Version = entry.minimum
			if err := VerifyTargetFeatures(t.Context(), api, harness.Profile{}, []Spec{selected}); err != nil {
				t.Fatalf("minimum supported target version rejected: %v", err)
			}
		})
	}
	if ok, err := featureVersionAtLeast("3.9", "3.25"); err != nil || ok {
		t.Fatal("REST version comparison used decimal ordering")
	}
}

func TestTargetFeatureProbesAreBoundedReadonlyAndDeduplicated(t *testing.T) {
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		switch r.URL.Path {
		case "/api/metadata/graphql":
			if r.Method != "POST" {
				t.Error("Metadata service probe must be a query POST")
			}
			var request map[string]any
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
			}
			if request["query"] != "query TadxLiveFeatureProbe { __typename }" {
				t.Error("Metadata feature probe fetched content or used a mutation")
			}
			fmt.Fprint(w, `{"data":{"__typename":"Query"}}`)
		case "/api/-/pulse/definitions":
			if r.Method != "GET" || r.URL.Query().Get("page_size") != "1" {
				t.Error("Pulse feature probe was not bounded read-only")
			}
			fmt.Fprint(w, `{"definitions":[]}`)
		case "/api/3.29/sites/site/labelCategories":
			if r.Method != "GET" {
				t.Error("label feature probe mutated target")
			}
			fmt.Fprint(w, `{"labelCategoryList":{"labelCategory":[]}}`)
		default:
			t.Errorf("unrequested feature endpoint %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	api := &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
	specs := []Spec{{Requires: []string{"metadata", "pulse", "labels-api"}}, {Requires: []string{"catalog-read", "pulse", "labels"}}}
	if err := VerifyTargetFeatures(t.Context(), api, harness.Profile{}, specs); err != nil {
		t.Fatal(err)
	}
	if len(counts) != 3 {
		t.Fatalf("unexpected feature probe count %v", counts)
	}
	for endpoint, count := range counts {
		if count != 1 {
			t.Fatalf("feature %s probed %d times", endpoint, count)
		}
	}
}

func TestMissingOptionalServiceIsUnsupportedButPermissionAndProtocolErrorsAreFailures(t *testing.T) {
	for _, service := range []string{"metadata", "pulse", "labels-api"} {
		for _, status := range []int{404, 401, 403, 500} {
			t.Run(fmt.Sprintf("%s/%d", service, status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
				defer server.Close()
				api := &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
				err := VerifyTargetFeatures(t.Context(), api, harness.Profile{Features: map[string]string{service: "enabled"}}, []Spec{{Requires: []string{service}}})
				if status == 404 {
					if harness.ErrorStatus(err) != harness.UnsupportedStatus {
						t.Fatalf("missing service did not produce unsupported: %v", err)
					}
				} else {
					if err == nil || harness.ErrorStatus(err) != harness.Failed {
						t.Fatalf("permission or service failure was hidden as unsupported/pass: %v", err)
					}
					if upstream, ok := errors.AsType[*harness.HTTPError](err); !ok || upstream.Status != status {
						t.Fatal("upstream failure identity was discarded")
					}
				}
			})
		}
	}
	for _, service := range []string{"metadata", "pulse", "labels-api"} {
		t.Run(service+"/malformed", func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
			defer server.Close()
			api := &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}
			if err := VerifyTargetFeatures(t.Context(), api, harness.Profile{}, []Spec{{Requires: []string{service}}}); err == nil || harness.ErrorStatus(err) != harness.Failed {
				t.Fatalf("empty unrelated JSON proved feature support: %v", err)
			}
		})
	}
}

func TestMetadataGraphQLErrorsDoNotCountAsSupportedFeature(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"__typename":"Query"},"errors":[{"message":"provider private detail"}]}`)
	}))
	defer server.Close()
	api := &harness.Client{Server: server.URL, HTTP: server.Client()}
	if err := VerifyTargetFeatures(t.Context(), api, harness.Profile{}, []Spec{{Requires: []string{"metadata"}}}); err == nil || harness.ErrorStatus(err) != harness.Failed {
		t.Fatalf("partial GraphQL failure proved feature support: %v", err)
	}
}

func TestFeatureProbeHonorsCallerCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	api := &harness.Client{Server: server.URL, HTTP: server.Client()}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := VerifyTargetFeatures(ctx, api, harness.Profile{Budgets: harness.Budgets{ObservationSeconds: 1}}, []Spec{{Requires: []string{"pulse"}}}); err == nil {
		t.Fatal("cancelled feature check passed")
	}
	if time.Since(started) > time.Second {
		t.Fatal("feature probe ignored caller deadline")
	}
}
