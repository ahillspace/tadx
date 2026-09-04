package list_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	page  flowlist.Page
	input flowlist.PageRequest
	calls int
}

func (r *reader) ListFlows(_ context.Context, input flowlist.PageRequest) (flowlist.Page, error) {
	r.input = input
	r.calls++
	return r.page, nil
}

func TestOutputGolden(t *testing.T) {
	output := flowlist.Output{Status: "listed", Environment: "dev", Site: "sandbox", Page: flowlist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "next-page"}, Flows: []flowlist.Flow{{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectName: "Ops", FileType: "tflx", Description: "Daily prep", OwnerLUID: "user-1", Tags: []string{"daily"}}}, RequestID: "request-1", Help: []string{"tadx content flow inspect --id <flow-luid>"}}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func TestFullOutputBoundsTagsPerFlow(t *testing.T) {
	tags := make([]string, 51)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	full := (flowlist.Output{Flows: []flowlist.Flow{{Tags: tags}}}).FullOutput().(flowlist.FullResult)
	if len(full.Flows[0].Tags) != 50 || full.Flows[0].TagsOmitted != 1 {
		t.Fatalf("full = %#v", full)
	}
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func TestActionListsCompactAndFullFlowDetails(t *testing.T) {
	output, err := flowlist.New(&reader{page: flowlist.Page{Number: 1, Size: 25, Total: 1, Flows: []flowlist.Flow{{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectName: "Ops", FileType: "tflx", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Prep", OwnerLUID: "u-1"}}}}).Execute(context.Background(), flowlist.Input{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(flowlist.CompactResult)
	if compact.Flows[0].LUID != "f-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	if output.FullOutput().(flowlist.FullResult).Flows[0].Description != "Prep" {
		t.Fatalf("full = %#v", output.FullOutput())
	}
}

func TestActionCursorIsBoundToFlowFilters(t *testing.T) {
	firstReader := &reader{page: flowlist.Page{Number: 1, Size: 2, Total: 3, Flows: make([]flowlist.Flow, 2)}}
	first, err := flowlist.New(firstReader).Execute(context.Background(), flowlist.Input{
		Environment: "dev",
		Limit:       2,
		Name:        "Private flow name",
		OwnerName:   "Private owner name",
		ProjectLUID: "project-1",
		ProjectName: "Private project name",
	})
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(first.Page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"Private flow name", "Private owner name", "Private project name"} {
		if strings.Contains(string(decoded), raw) {
			t.Fatalf("cursor exposes raw filter %q: %s", raw, decoded)
		}
	}

	tests := []struct {
		name   string
		mutate func(*flowlist.Input)
	}{
		{name: "name", mutate: func(input *flowlist.Input) { input.Name = "Other" }},
		{name: "owner", mutate: func(input *flowlist.Input) { input.OwnerName = "Other" }},
		{name: "project LUID", mutate: func(input *flowlist.Input) { input.ProjectLUID = "project-2" }},
		{name: "project name", mutate: func(input *flowlist.Input) { input.ProjectName = "Other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := flowlist.Input{Environment: "dev", Cursor: first.Page.NextCursor, Limit: 2, Name: "Private flow name", OwnerName: "Private owner name", ProjectLUID: "project-1", ProjectName: "Private project name"}
			test.mutate(&input)
			continuationReader := &reader{}
			_, err := flowlist.New(continuationReader).Execute(context.Background(), input)
			if err == nil || err.Error() != "flow continuation cursor does not match the current filters" {
				t.Fatalf("error = %v", err)
			}
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
				t.Fatalf("error kind = %#v", structured)
			}
			if continuationReader.calls != 0 {
				t.Fatalf("reader calls = %d", continuationReader.calls)
			}
		})
	}
}

func TestActionCursorIsBoundToResolvedFlowEnvironment(t *testing.T) {
	firstReader := &reader{page: flowlist.Page{Number: 1, Size: 1, Total: 2, Flows: []flowlist.Flow{{LUID: "f-1"}}}}
	first, err := flowlist.New(firstReader).Execute(context.Background(), flowlist.Input{Environment: "dev", Site: "site-a", Limit: 1, Name: "Daily"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []flowlist.Input{
		{Environment: "production", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Daily"},
		{Environment: "dev", Site: "site-b", Cursor: first.Page.NextCursor, Name: "Daily"},
	} {
		continuationReader := &reader{}
		_, err = flowlist.New(continuationReader).Execute(context.Background(), input)
		if err == nil || err.Error() != "flow continuation cursor does not match the current filters" {
			t.Fatalf("error = %v", err)
		}
		if continuationReader.calls != 0 {
			t.Fatalf("reader calls = %d", continuationReader.calls)
		}
	}
}

func TestActionRejectsFlowCursorVersionAndLimitMismatch(t *testing.T) {
	legacy := base64.RawURLEncoding.EncodeToString([]byte(`{"Page":2,"Size":2}`))
	_, err := flowlist.New(&reader{}).Execute(context.Background(), flowlist.Input{Cursor: legacy})
	assertUsageError(t, err, "invalid flow continuation cursor")

	first, err := flowlist.New(&reader{page: flowlist.Page{Number: 1, Size: 2, Total: 3, Flows: make([]flowlist.Flow, 2)}}).Execute(context.Background(), flowlist.Input{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	continuationReader := &reader{}
	_, err = flowlist.New(continuationReader).Execute(context.Background(), flowlist.Input{Cursor: first.Page.NextCursor, Limit: 3})
	assertUsageError(t, err, "flow list limit must match the continuation cursor")
	if continuationReader.calls != 0 {
		t.Fatalf("reader calls = %d", continuationReader.calls)
	}
}

func assertUsageError(t *testing.T, err error, message string) {
	t.Helper()
	if err == nil || err.Error() != message {
		t.Fatalf("error = %v", err)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}
