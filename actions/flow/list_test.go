package flow_test

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

	flowlist "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type listReader struct {
	page  flowlist.ListPage
	input flowlist.ListPageRequest
	calls int
}

func (r *listReader) ListFlows(_ context.Context, input flowlist.ListPageRequest) (flowlist.ListPage, error) {
	r.input = input
	r.calls++
	return r.page, nil
}

func TestListOutputGolden(t *testing.T) {
	output := flowlist.ListOutput{Status: "listed", Environment: "dev", Site: "sandbox", Page: flowlist.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "next-page"}, Flows: []flowlist.Record{{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectName: "Ops", FileType: "tflx", Description: "Daily prep", OwnerLUID: "user-1", Tags: []string{"daily"}}}, RequestID: "request-1", Help: []string{"tadx content flow inspect --id <flow-luid>"}}
	listAssertGolden(t, "compact.toon", output, false)
	listAssertGolden(t, "full.toon", output, true)
}

func TestListFullOutputBoundsTagsPerFlow(t *testing.T) {
	tags := make([]string, 51)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	full := (flowlist.ListOutput{Flows: []flowlist.Record{{Tags: tags}}}).FullOutput().(flowlist.ListFullResult)
	if len(full.Flows[0].Tags) != 50 || full.Flows[0].TagsOmitted != 1 {
		t.Fatalf("full = %#v", full)
	}
}

func listAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "list", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func TestListActionListsCompactAndFullFlowDetails(t *testing.T) {
	output, err := flowlist.List(context.Background(), &listReader{page: flowlist.ListPage{Number: 1, Size: 25, Total: 1, Flows: []flowlist.Record{{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectName: "Ops", FileType: "tflx", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Prep", OwnerLUID: "u-1"}}}}, flowlist.ListInput{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(flowlist.ListCompactResult)
	if compact.Flows[0].LUID != "f-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	if output.FullOutput().(flowlist.ListFullResult).Flows[0].Description != "Prep" {
		t.Fatalf("full = %#v", output.FullOutput())
	}
}

func TestListActionCursorIsBoundToFlowFilters(t *testing.T) {
	firstReader := &listReader{page: flowlist.ListPage{Number: 1, Size: 2, Total: 3, Flows: make([]flowlist.Record, 2)}}
	first, err := flowlist.List(context.Background(), firstReader, flowlist.ListInput{
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
		mutate func(*flowlist.ListInput)
	}{
		{name: "name", mutate: func(input *flowlist.ListInput) { input.Name = "Other" }},
		{name: "owner", mutate: func(input *flowlist.ListInput) { input.OwnerName = "Other" }},
		{name: "project LUID", mutate: func(input *flowlist.ListInput) { input.ProjectLUID = "project-2" }},
		{name: "project name", mutate: func(input *flowlist.ListInput) { input.ProjectName = "Other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := flowlist.ListInput{Environment: "dev", Cursor: first.Page.NextCursor, Limit: 2, Name: "Private flow name", OwnerName: "Private owner name", ProjectLUID: "project-1", ProjectName: "Private project name"}
			test.mutate(&input)
			continuationReader := &listReader{}
			_, err := flowlist.List(context.Background(), continuationReader, input)
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

func TestListActionCursorIsBoundToResolvedFlowEnvironment(t *testing.T) {
	firstReader := &listReader{page: flowlist.ListPage{Number: 1, Size: 1, Total: 2, Flows: []flowlist.Record{{LUID: "f-1"}}}}
	first, err := flowlist.List(context.Background(), firstReader, flowlist.ListInput{Environment: "dev", Site: "site-a", Limit: 1, Name: "Daily"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []flowlist.ListInput{
		{Environment: "production", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Daily"},
		{Environment: "dev", Site: "site-b", Cursor: first.Page.NextCursor, Name: "Daily"},
	} {
		continuationReader := &listReader{}
		_, err = flowlist.List(context.Background(), continuationReader, input)
		if err == nil || err.Error() != "flow continuation cursor does not match the current filters" {
			t.Fatalf("error = %v", err)
		}
		if continuationReader.calls != 0 {
			t.Fatalf("reader calls = %d", continuationReader.calls)
		}
	}
}

func TestListActionRejectsFlowCursorVersionAndLimitMismatch(t *testing.T) {
	legacy := base64.RawURLEncoding.EncodeToString([]byte(`{"Page":2,"Size":2}`))
	_, err := flowlist.List(context.Background(), &listReader{}, flowlist.ListInput{Cursor: legacy})
	listAssertUsageError(t, err, "invalid flow continuation cursor")

	first, err := flowlist.List(context.Background(), &listReader{page: flowlist.ListPage{Number: 1, Size: 2, Total: 3, Flows: make([]flowlist.Record, 2)}}, flowlist.ListInput{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	continuationReader := &listReader{}
	_, err = flowlist.List(context.Background(), continuationReader, flowlist.ListInput{Cursor: first.Page.NextCursor, Limit: 3})
	listAssertUsageError(t, err, "flow list limit must match the continuation cursor")
	if continuationReader.calls != 0 {
		t.Fatalf("reader calls = %d", continuationReader.calls)
	}
}

func listAssertUsageError(t *testing.T, err error, message string) {
	t.Helper()
	if err == nil || err.Error() != message {
		t.Fatalf("error = %v", err)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}
