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

	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	page  workbooklist.Page
	input workbooklist.PageRequest
	calls int
}

func (r *reader) ListWorkbooks(_ context.Context, input workbooklist.PageRequest) (workbooklist.Page, error) {
	r.input = input
	r.calls++
	return r.page, nil
}

func TestOutputGolden(t *testing.T) {
	output := workbooklist.Output{
		Status: "listed", Environment: "dev", Site: "sandbox",
		Page:      workbooklist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "next-page"},
		Workbooks: []workbooklist.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Tags: []string{"finance"}}},
		RequestID: "request-1", Help: []string{"tadx content workbook get --id <workbook-luid>"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func TestFullOutputBoundsTagsPerWorkbook(t *testing.T) {
	tags := make([]string, 51)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	full := (workbooklist.Output{Workbooks: []workbooklist.Workbook{{Tags: tags}}}).FullOutput().(workbooklist.FullResult)
	if len(full.Workbooks[0].Tags) != 50 || full.Workbooks[0].TagsOmitted != 1 {
		t.Fatalf("full = %#v", full)
	}
}

func TestActionForwardsWorkbookFiltersAndRequestID(t *testing.T) {
	r := &reader{page: workbooklist.Page{Number: 1, Size: 25, Total: 1, Workbooks: []workbooklist.Workbook{{LUID: "wb-1"}}, RequestID: "request-1"}}
	output, err := workbooklist.New(r).Execute(context.Background(), workbooklist.Input{Environment: "dev", Site: "site-a", Name: "Finance", OwnerName: "Analyst", ProjectName: "Ops", Tag: "quarterly"})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.PageNumber != 1 || r.input.PageSize != 25 || r.input.Name != "Finance" || r.input.OwnerName != "Analyst" || r.input.ProjectName != "Ops" || r.input.Tag != "quarterly" {
		t.Fatalf("request = %#v", r.input)
	}
	if output.RequestID != "request-1" || output.Page.Returned != 1 || output.Page.Limit != 25 {
		t.Fatalf("output = %#v", output)
	}
}

func TestActionCursorIsBoundToEveryWorkbookFilterAndTarget(t *testing.T) {
	firstReader := &reader{page: workbooklist.Page{Number: 1, Size: 2, Total: 3, Workbooks: make([]workbooklist.Workbook, 2)}}
	first, err := workbooklist.New(firstReader).Execute(context.Background(), workbooklist.Input{
		Environment: "dev", Site: "site-a", Limit: 2, Name: "Private workbook", OwnerName: "Private owner",
		ProjectName: "Private project", Tag: "Private tag",
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first.Page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"Private workbook", "Private owner", "Private project", "Private tag"} {
		if strings.Contains(string(decoded), raw) {
			t.Fatalf("cursor exposes raw filter %q: %s", raw, decoded)
		}
	}
	tests := []struct {
		name   string
		mutate func(*workbooklist.Input)
	}{
		{name: "environment", mutate: func(input *workbooklist.Input) { input.Environment = "prod" }},
		{name: "site", mutate: func(input *workbooklist.Input) { input.Site = "site-b" }},
		{name: "name", mutate: func(input *workbooklist.Input) { input.Name = "Other" }},
		{name: "owner", mutate: func(input *workbooklist.Input) { input.OwnerName = "Other" }},
		{name: "project name", mutate: func(input *workbooklist.Input) { input.ProjectName = "Other" }},
		{name: "tag", mutate: func(input *workbooklist.Input) { input.Tag = "Other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := workbooklist.Input{Environment: "dev", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Private workbook", OwnerName: "Private owner", ProjectName: "Private project", Tag: "Private tag"}
			test.mutate(&input)
			continuationReader := &reader{}
			_, err := workbooklist.New(continuationReader).Execute(context.Background(), input)
			if err == nil || err.Error() != "workbook continuation cursor does not match the current filters" {
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

func TestActionRejectsCursorWithAbsurdPageNumber(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"p":999999999999,"s":25,"f":"x"}`))
	r := &reader{}
	_, err := workbooklist.New(r).Execute(context.Background(), workbooklist.Input{Environment: "dev", Site: "site", Cursor: cursor})
	if err == nil || err.Error() != "invalid workbook continuation cursor" || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func TestActionRejectsOutOfRangeLimitAsUsage(t *testing.T) {
	r := &reader{}
	_, err := workbooklist.New(r).Execute(context.Background(), workbooklist.Input{Environment: "dev", Site: "site", Limit: 500})
	if err == nil || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
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
