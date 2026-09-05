package list_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
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
	for _, input := range []capabilitylist.Input{{Limit: -1}, {Limit: 101}, {Cursor: "nope"}} {
		if _, err := capabilitylist.New(source{}).Execute(context.Background(), input); err == nil {
			t.Fatalf("Execute(%#v) error = nil", input)
		}
	}
}

func TestOutputGoldenIsBoundedAndUsesPlaceholders(t *testing.T) {
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
	if !strings.Contains(rendered.String(), "<id>") || strings.Contains(rendered.String(), "capability get content.datasource.list") {
		t.Fatalf("help must use an explicit placeholder: %s", rendered.String())
	}
}
