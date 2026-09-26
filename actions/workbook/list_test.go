package workbook_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type listReader struct {
	page  workbookops.ListPage
	input workbookops.ListPageRequest
	calls int
}

func (r *listReader) ListWorkbooks(_ context.Context, input workbookops.ListPageRequest) (workbookops.ListPage, error) {
	r.input = input
	r.calls++
	return r.page, nil
}

func TestListOutputGolden(t *testing.T) {
	output := workbookops.ListOutput{
		Status: "listed", Environment: "dev", Site: "sandbox",
		Page:      workbookops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "next-page"},
		Workbooks: []workbookops.Record{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Department/Ops", ContentURL: "Finance", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Finance reporting", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z", Tags: []string{"finance"}}},
		RequestID: "request-1", Help: []string{"tadx content workbook inspect --id <workbook-luid>"},
	}
	listAssertGolden(t, "compact.toon", output, false)
	listAssertGolden(t, "full.toon", output, true)
}

func TestListFullOutputBoundsTagsPerWorkbook(t *testing.T) {
	tags := make([]string, 51)
	for index := range tags {
		tags[index] = fmt.Sprintf("tag-%02d", index)
	}
	full := (workbookops.ListOutput{Workbooks: []workbookops.Record{{Tags: tags}}}).FullOutput().(workbookops.ListFullResult)
	if len(full.Workbooks[0].Tags) != 50 || full.Workbooks[0].TagsOmitted != 1 {
		t.Fatalf("full = %#v", full)
	}
}

func TestListActionForwardsWorkbookFiltersAndRequestID(t *testing.T) {
	r := &listReader{page: workbookops.ListPage{Number: 1, Size: 25, Total: 1, Workbooks: []workbookops.Record{{LUID: "wb-1"}}, RequestID: "request-1"}}
	output, err := workbookops.List(context.Background(), r, workbookops.ListInput{Environment: "dev", Site: "site-a", Name: "Finance", OwnerName: "Analyst", ProjectLUID: "project-1", ProjectName: "Ops", Tag: "quarterly"})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.PageNumber != 1 || r.input.PageSize != 25 || r.input.Name != "Finance" || r.input.OwnerName != "Analyst" || r.input.ProjectLUID != "project-1" || r.input.ProjectName != "Ops" || r.input.Tag != "quarterly" {
		t.Fatalf("request = %#v", r.input)
	}
	if output.RequestID != "request-1" || output.Page.Returned != 1 || output.Page.Limit != 25 {
		t.Fatalf("output = %#v", output)
	}
}

func TestListActionCursorIsBoundToEveryWorkbookFilterAndTarget(t *testing.T) {
	firstReader := &listReader{page: workbookops.ListPage{Number: 1, Size: 2, Total: 3, Workbooks: make([]workbookops.Record, 2)}}
	first, err := workbookops.List(context.Background(), firstReader, workbookops.ListInput{
		Environment: "dev", Site: "site-a", Limit: 2, Name: "Private workbook", OwnerName: "Private owner", ProjectLUID: "project-1",
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
	continuationReader := &listReader{page: workbookops.ListPage{Number: 2, Size: 2, Total: 3, Workbooks: []workbookops.Record{{LUID: "wb-3"}}}}
	continued, err := workbookops.List(t.Context(), continuationReader, workbookops.ListInput{
		Environment: "dev", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Private workbook", OwnerName: "Private owner",
		ProjectLUID: "project-1", ProjectName: "Private project", Tag: "Private tag",
	})
	if err != nil || continuationReader.calls != 1 || continuationReader.input.PageNumber != 2 || continuationReader.input.PageSize != 2 {
		t.Fatalf("continuation request = %#v, calls = %d, error = %v", continuationReader.input, continuationReader.calls, err)
	}
	if continued.Page.NextCursor != "" || continued.Page.MoreAvailable || continued.Page.Returned != 1 || continued.Workbooks[0].LUID != "wb-3" {
		t.Fatalf("continued output = %#v", continued)
	}
	tests := []struct {
		name   string
		mutate func(*workbookops.ListInput)
	}{
		{name: "environment", mutate: func(input *workbookops.ListInput) { input.Environment = "prod" }},
		{name: "site", mutate: func(input *workbookops.ListInput) { input.Site = "site-b" }},
		{name: "name", mutate: func(input *workbookops.ListInput) { input.Name = "Other" }},
		{name: "owner", mutate: func(input *workbookops.ListInput) { input.OwnerName = "Other" }},
		{name: "project LUID", mutate: func(input *workbookops.ListInput) { input.ProjectLUID = "project-2" }},
		{name: "project name", mutate: func(input *workbookops.ListInput) { input.ProjectName = "Other" }},
		{name: "tag", mutate: func(input *workbookops.ListInput) { input.Tag = "Other" }},
		{name: "cache", mutate: func(input *workbookops.ListInput) { input.Cache = true }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := workbookops.ListInput{Environment: "dev", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Private workbook", OwnerName: "Private owner", ProjectLUID: "project-1", ProjectName: "Private project", Tag: "Private tag"}
			test.mutate(&input)
			continuationReader := &listReader{}
			_, err := workbookops.List(context.Background(), continuationReader, input)
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

func TestListActionRejectsCursorWithAbsurdPageNumber(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"p":999999999999,"s":25,"f":"x"}`))
	r := &listReader{}
	_, err := workbookops.List(context.Background(), r, workbookops.ListInput{Environment: "dev", Site: "site", Cursor: cursor})
	if err == nil || err.Error() != "invalid workbook continuation cursor" || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func TestListActionRejectsOutOfRangeLimitAsUsage(t *testing.T) {
	r := &listReader{}
	_, err := workbookops.List(context.Background(), r, workbookops.ListInput{Environment: "dev", Site: "site", Limit: 10001})
	if err == nil || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func listAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/list", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}
