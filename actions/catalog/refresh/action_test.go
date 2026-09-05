package refresh_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	refresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type recordingHydrator struct {
	requests []refresh.HydrationRequest
	result   refresh.HydrationResult
	err      error
}

type retryableHydrationError struct{}

func (retryableHydrationError) Error() string            { return "Tableau is unavailable" }
func (retryableHydrationError) Retryable() bool          { return true }
func (retryableHydrationError) CorrectiveAction() string { return "Retry after Tableau recovers." }
func (retryableHydrationError) RequestID() string        { return "request-1" }

func (h *recordingHydrator) Hydrate(_ context.Context, request refresh.HydrationRequest) (refresh.HydrationResult, error) {
	h.requests = append(h.requests, request)
	result := h.result
	if result.RequestedScopes == nil {
		result.RequestedScopes = append([]string(nil), request.RequestedScopes...)
	}
	if result.ImplicitScopes == nil {
		result.ImplicitScopes = append([]string(nil), request.ImplicitScopes...)
	}
	return result, h.err
}

func completeResult() refresh.HydrationResult {
	return refresh.HydrationResult{
		GenerationID:        "generation-1",
		GeneratedAt:         time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		Complete:            true,
		Source:              "tableau-rest",
		Path:                "catalog/catalog.sqlite",
		RecordCount:         3,
		HydratedRecordCount: 3,
		ScopeCounts:         []refresh.ScopeCount{{Scope: "projects", Records: 1}, {Scope: "workbooks", Records: 2}},
		Diagnostics:         refresh.Diagnostics{Requests: 2, FailedRequests: 0, Duration: "1.211s"},
	}
}

func TestActionDefaultsToFullHydration(t *testing.T) {
	hydrator := &recordingHydrator{result: completeResult()}
	if _, err := refresh.New(hydrator).Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions"}
	if len(hydrator.requests) != 1 || !reflect.DeepEqual(hydrator.requests[0].RequestedScopes, want) || len(hydrator.requests[0].ImplicitScopes) != 0 {
		t.Fatalf("hydration requests = %#v", hydrator.requests)
	}
}

func TestActionRequiresResolvedEnvironmentAndSiteBeforeHydration(t *testing.T) {
	for _, input := range []refresh.Input{
		{Site: "marketing", SiteResolved: true},
		{Environment: "production", Site: "marketing", SiteResolved: false},
	} {
		hydrator := &recordingHydrator{result: completeResult()}
		_, err := refresh.New(hydrator).Execute(context.Background(), input)
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.ID != "catalog.refresh.usage" || len(hydrator.requests) != 0 {
			t.Fatalf("input = %#v, error = %#v, requests = %#v", input, err, hydrator.requests)
		}
	}
}

func TestActionNormalizesRequestedScopesAndDependencyClosure(t *testing.T) {
	hydrator := &recordingHydrator{result: completeResult()}
	_, err := refresh.New(hydrator).Execute(context.Background(), refresh.Input{
		Environment: "production", Site: "marketing", SiteResolved: true,
		Scopes: []string{"permissions", "views", "datasources"},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := hydrator.requests[0]
	if want := []string{"datasources", "views", "permissions"}; !reflect.DeepEqual(request.RequestedScopes, want) {
		t.Fatalf("requested scopes = %v, want %v", request.RequestedScopes, want)
	}
	if want := []string{"projects", "workbooks"}; !reflect.DeepEqual(request.ImplicitScopes, want) {
		t.Fatalf("implicit scopes = %v, want %v", request.ImplicitScopes, want)
	}
}

func TestActionRejectsDuplicateAndUnknownScopesBeforeHydration(t *testing.T) {
	for _, test := range []struct {
		name   string
		scopes []string
	}{
		{name: "duplicate", scopes: []string{"users", "users"}},
		{name: "unknown", scopes: []string{"subscriptions"}},
		{name: "case mismatch", scopes: []string{"Users"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			hydrator := &recordingHydrator{result: completeResult()}
			_, err := refresh.New(hydrator).Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true, Scopes: test.scopes})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.ID != "catalog.refresh.usage" || structured.Kind != errs.KindUsage {
				t.Fatalf("Execute() error = %#v", err)
			}
			if len(hydrator.requests) != 0 {
				t.Fatalf("hydrator called with %#v", hydrator.requests)
			}
		})
	}
}

func TestActionRejectsIncompleteHydrationReceipt(t *testing.T) {
	result := completeResult()
	result.Complete = false
	_, err := refresh.New(&recordingHydrator{result: result}).Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "catalog.refresh.incomplete" {
		t.Fatalf("Execute() error = %#v", err)
	}
}

func TestActionRejectsInvalidHydrationReceipts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*refresh.HydrationResult)
	}{
		{name: "failed requests", mutate: func(result *refresh.HydrationResult) { result.Diagnostics.FailedRequests = 1 }},
		{name: "absolute path", mutate: func(result *refresh.HydrationResult) { result.Path = `C:\catalog\catalog.sqlite` }},
		{name: "scope mismatch", mutate: func(result *refresh.HydrationResult) { result.RequestedScopes = []string{"users"} }},
		{name: "unordered counts", mutate: func(result *refresh.HydrationResult) {
			result.ScopeCounts = []refresh.ScopeCount{{Scope: "workbooks", Records: 2}, {Scope: "projects", Records: 1}}
		}},
		{name: "unselected count", mutate: func(result *refresh.HydrationResult) {
			result.ScopeCounts = []refresh.ScopeCount{{Scope: "users", Records: 3}}
		}},
		{name: "inconsistent total", mutate: func(result *refresh.HydrationResult) {
			result.ScopeCounts = []refresh.ScopeCount{{Scope: "projects", Records: 1}, {Scope: "workbooks", Records: 1}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := completeResult()
			test.mutate(&result)
			_, err := refresh.New(&recordingHydrator{result: result}).Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"workbooks"}})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.ID != "catalog.refresh.incomplete" {
				t.Fatalf("Execute() error = %#v", err)
			}
		})
	}
}

func TestActionPreservesHydrationRetryAdviceAndRequestID(t *testing.T) {
	_, err := refresh.New(&recordingHydrator{err: retryableHydrationError{}}).Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "catalog.refresh.failed" || structured.Retryable == nil || !*structured.Retryable || structured.CorrectiveAction != "Retry after Tableau recovers." || structured.TableauRequestID != "request-1" {
		t.Fatalf("Execute() error = %#v", err)
	}
}

func TestActionCompactAndFullOutputContainOnlyBoundedOperationalMetadata(t *testing.T) {
	result := completeResult()
	result.Warnings = []string{"one workbook permission request was forbidden"}
	action := refresh.New(&recordingHydrator{result: result})
	for _, test := range []struct {
		name, golden string
		full         bool
	}{{"compact", "testdata/compact.toon", false}, {"full", "testdata/full.toon", true}} {
		t.Run(test.name, func(t *testing.T) {
			value, err := action.Execute(context.Background(), refresh.Input{Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"workbooks"}})
			if err != nil {
				t.Fatal(err)
			}
			actual := render(t, value, test.full)
			for _, forbidden := range []string{"items[", "records[", "wb-1", "Finance"} {
				if strings.Contains(actual, forbidden) {
					t.Fatalf("output contains hydrated catalog content %q:\n%s", forbidden, actual)
				}
			}
			assertGolden(t, test.golden, actual)
		})
	}
}

func TestActionSeparatesSearchableRecordsFromWiderHydrationCounts(t *testing.T) {
	result := completeResult()
	result.RecordCount = 2
	result.HydratedRecordCount = 3
	hydrator := &recordingHydrator{result: result}

	value, err := refresh.New(hydrator).Execute(context.Background(), refresh.Input{
		Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"workbooks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value.Generation.Records != 2 || value.Generation.HydratedRecords != 3 {
		t.Fatalf("generation = %#v", value.Generation)
	}
	compact, err := json.Marshal(value.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compact), "hydrated_records") {
		t.Fatalf("compact output exposed diagnostic hydration count: %s", compact)
	}
	full, err := json.Marshal(value.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(full), `"records":2`) || !strings.Contains(string(full), `"hydrated_records":3`) {
		t.Fatalf("full output did not distinguish catalog and hydration counts: %s", full)
	}
}

func render(t *testing.T, value any, full bool) string {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	return actual.String()
}

func assertGolden(t *testing.T, path, actual string) {
	t.Helper()
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(actual), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual)
	}
}
