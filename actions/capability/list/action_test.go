package list_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type source struct {
	items []capabilitylist.Capability
	err   error
}

func (s source) List(context.Context) ([]capabilitylist.Capability, error) {
	return append([]capabilitylist.Capability(nil), s.items...), s.err
}

func TestExecuteSortsAndReturnsBoundedDiscovery(t *testing.T) {
	action := capabilitylist.New(source{items: []capabilitylist.Capability{
		{ID: "workbook.publish", Owner: "cli", State: "planned"},
		{ID: "capability.get", Owner: "cli", State: "implemented", Command: "capability get"},
	}})

	got, err := action.Execute(context.Background(), capabilitylist.Input{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantIDs := []string{"capability.get", "workbook.publish"}
	gotIDs := []string{got.Capabilities[0].ID, got.Capabilities[1].ID}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("IDs = %v, want %v", gotIDs, wantIDs)
	}
	if got.Page.Returned != 2 || got.Page.Total != 2 {
		t.Fatalf("Page = %#v, want returned and total 2", got.Page)
	}
	if len(got.Help) == 0 {
		t.Fatal("Help is empty")
	}
}

func TestExecuteGuardsUnconfiguredSourceWithoutPanic(t *testing.T) {
	for name, action := range map[string]*capabilitylist.Action{
		"nil action": nil,
		"nil source": capabilitylist.New(nil),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := action.Execute(context.Background(), capabilitylist.Input{})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindRuntime || structured.ID != "capability.list.unconfigured" {
				t.Fatalf("Execute() error = %#v", err)
			}
		})
	}
}

func TestExecuteReturnsDefinitiveEmptyState(t *testing.T) {
	got, err := capabilitylist.New(source{}).Execute(context.Background(), capabilitylist.Input{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Page.Returned != 0 || got.Page.Total != 0 || got.Capabilities == nil {
		t.Fatalf("empty output = %#v", got)
	}
}

func TestExecuteFiltersAndPaginatesDeterministically(t *testing.T) {
	action := capabilitylist.New(source{items: []capabilitylist.Capability{
		{ID: "content.workbook.list", Owner: "cli", Domain: "content", Resource: "workbook", Product: "cloud/server"},
		{ID: "content.datasource.list", Owner: "cli", Domain: "content", Resource: "datasource", Product: "cloud/server"},
		{ID: "pulse.metric.list", Owner: "cli", Domain: "pulse", Resource: "metric", Product: "pulse"},
	}})

	got, err := action.Execute(context.Background(), capabilitylist.Input{Domain: "content", Owner: "cli", Product: "cloud", Limit: 1})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Page.Returned != 1 || got.Page.Total != 2 || got.Page.NextCursor != "1" {
		t.Fatalf("Page = %#v", got.Page)
	}
	if got.Capabilities[0].ID != "content.datasource.list" {
		t.Fatalf("first ID = %q", got.Capabilities[0].ID)
	}

	got, err = action.Execute(context.Background(), capabilitylist.Input{Domain: "content", Cursor: got.Page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if got.Capabilities[0].ID != "content.workbook.list" || got.Page.NextCursor != "" {
		t.Fatalf("second page = %#v", got)
	}
}

func TestExecuteAllReturnsCompleteMatchingInventory(t *testing.T) {
	action := capabilitylist.New(source{items: []capabilitylist.Capability{
		{ID: "content.datasource.list", Domain: "content"},
		{ID: "content.workbook.list", Domain: "content"},
		{ID: "pulse.metric.list", Domain: "pulse"},
	}})

	got, err := action.Execute(context.Background(), capabilitylist.Input{All: true, Domain: "content"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Page.Returned != 2 || got.Page.Total != 2 || got.Page.Limit != capabilitylist.MaxLimit || got.Page.MoreAvailable || got.Page.NextCursor != "" {
		t.Fatalf("all page = %#v", got.Page)
	}
	if got.NextCommand != "" {
		t.Fatalf("all next command = %q", got.NextCommand)
	}
	if ids := []string{got.Capabilities[0].ID, got.Capabilities[1].ID}; !reflect.DeepEqual(ids, []string{"content.datasource.list", "content.workbook.list"}) {
		t.Fatalf("all IDs = %v", ids)
	}
}

func TestExecuteAllRejectsPaginationOverrides(t *testing.T) {
	for _, input := range []capabilitylist.Input{{All: true, Limit: 1}, {All: true, Cursor: "0"}} {
		if _, err := capabilitylist.New(source{}).Execute(context.Background(), input); err == nil {
			t.Fatalf("Execute(%#v) error = nil", input)
		}
	}
}

func TestExecuteAllFailsWhenMatchingInventoryExceedsBound(t *testing.T) {
	items := make([]capabilitylist.Capability, capabilitylist.MaxLimit+1)
	for index := range items {
		items[index].ID = "capability." + strconv.Itoa(index)
	}
	_, err := capabilitylist.New(source{items: items}).Execute(context.Background(), capabilitylist.Input{All: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || !strings.Contains(structured.Summary, "10000-record bound") {
		t.Fatalf("error = %#v, want bounded usage error", err)
	}
}

func TestExecuteMutationFilterDoesNotAuthorizeOrHideDiscovery(t *testing.T) {
	mutation := true
	got, err := capabilitylist.New(source{items: []capabilitylist.Capability{
		{ID: "workbook.publish", RemoteMutation: true},
		{ID: "workbook.inspect"},
	}}).Execute(context.Background(), capabilitylist.Input{Mutation: &mutation})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].ID != "workbook.publish" || got.Capabilities[0].ExecutionEnabled {
		t.Fatalf("capabilities = %#v", got.Capabilities)
	}

	got, err = capabilitylist.New(source{items: []capabilitylist.Capability{{ID: "workbook.publish", State: "implemented", RemoteMutation: true}}}).Execute(context.Background(), capabilitylist.Input{MutationsEnabled: true})
	if err != nil || len(got.Capabilities) != 1 || !got.Capabilities[0].ExecutionEnabled {
		t.Fatalf("enabled capabilities = %#v, error = %v", got.Capabilities, err)
	}
}

func TestExecuteRejectsInvalidPagination(t *testing.T) {
	for _, input := range []capabilitylist.Input{{Limit: -1}, {Limit: 10001}, {Cursor: "nope"}} {
		if _, err := capabilitylist.New(source{}).Execute(context.Background(), input); err == nil {
			t.Fatalf("Execute(%#v) error = nil", input)
		}
	}
}

func TestOutputGoldenIsBoundedAndUsesExactIdentity(t *testing.T) {
	action := capabilitylist.New(source{items: []capabilitylist.Capability{
		{ID: "content.workbook.list", Owner: "cli", Disposition: "ship", State: "planned", Domain: "content"},
		{ID: "content.datasource.list", Owner: "cli", Disposition: "ship", State: "planned", Domain: "content"},
	}})
	result, err := action.Execute(context.Background(), capabilitylist.Input{Domain: "content", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, result); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	wantText := string(want)
	if rendered.String() != wantText {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, rendered.String())
	}
	if rendered.Len() > 700 {
		t.Fatalf("bounded output grew to %d bytes", rendered.Len())
	}
	if strings.Contains(rendered.String(), "<id>") || !strings.Contains(rendered.String(), "capability get content.datasource.list") {
		t.Fatalf("help must use a returned exact identity: %s", rendered.String())
	}
}

func TestFullOutputRetainsGetContractForEveryReturnedRow(t *testing.T) {
	result, err := capabilitylist.New(source{items: []capabilitylist.Capability{
		{
			ID: "content.workbook.publish", Domain: "content", Resource: "workbook", Verb: "publish",
			Surface: "tadx content workbook publish", Outcome: "Publish a workbook.", OperationType: "deliver",
			Owner: "cli", Disposition: "ship", EvidenceLevel: "local-contract", VerificationReadiness: "ready",
			ImplementationState: "implemented", Command: "content workbook publish", Selectors: []string{"Workbook LUID; target"},
			Availability: "Cloud / Server", SafetyGuard: "Preview first", ArtifactEffect: "Remote workbook",
			UpstreamOperation: "POST /workbooks", Evidence: "local test", Validation: "identity checked",
			RemoteMutation: true, SupportsPreview: true,
		},
	}}).Execute(context.Background(), capabilitylist.Input{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	full, err := json.Marshal(result.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Capabilities []capabilitylist.Capability `json:"capabilities"`
	}
	if err := json.Unmarshal(full, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Capabilities) != 1 || document.Capabilities[0].Selectors[0] != "Workbook LUID; target" || document.Capabilities[0].SafetyGuard != "Preview first" {
		t.Fatalf("full capabilities = %#v", document.Capabilities)
	}
	compact, err := json.Marshal(result.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compact), "selectors") || !strings.Contains(string(compact), `"state":"implemented"`) {
		t.Fatalf("compact projection leaked or omitted fields: %s", compact)
	}
}

func TestContinuationPreservesFiltersAndPresentation(t *testing.T) {
	const environment = "qa team's $literal"
	items := []capabilitylist.Capability{
		{ID: "content.workbook.first", Domain: "content", Resource: "workbook", Owner: "cli", Availability: "Cloud"},
		{ID: "content.workbook.second", Domain: "content", Resource: "workbook", Owner: "cli", Availability: "Cloud"},
	}
	result, err := capabilitylist.New(source{items: items}).Execute(t.Context(), capabilitylist.Input{
		Environment: environment,
		Domain:      "content", Resource: "workbook", Owner: "cli", Product: "cloud", Limit: 1, Full: true, JSON: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := commandhint.Environment(environment, "capability", "list", "--domain", "content", "--resource", "workbook", "--owner", "cli", "--product", "cloud", "--cursor", "1", "--limit", "1", "--full", "--json")
	if result.NextCommand != want {
		t.Fatalf("next_command = %q", result.NextCommand)
	}
	if len(result.Help) != 2 || result.Help[0] != commandhint.Environment(environment, "capability", "get", items[0].ID) || result.Help[1] != "When more results are needed, use next_command." {
		t.Fatalf("help = %#v", result.Help)
	}
}

func TestCountsKeepCombinedPageAndOutOfScopeRowsDistinct(t *testing.T) {
	result, err := capabilitylist.New(source{items: []capabilitylist.Capability{
		{ID: "a.local", Disposition: "ship"},
		{ID: "b.external", Disposition: "delegated"},
	}}).Execute(context.Background(), capabilitylist.Input{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Page.Returned != 2 || result.Page.Total != 2 || result.Counts.Returned != 2 || result.Counts.Matched != 2 || result.Counts.OutOfScope != 1 {
		t.Fatalf("page/counts = %#v / %#v", result.Page, result.Counts)
	}
	compact := result.CompactOutput()
	encoded, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"out_of_scope"`) || strings.Contains(string(encoded), `"id":"b.external","owner"`) {
		t.Fatalf("compact projection = %s", encoded)
	}
}
