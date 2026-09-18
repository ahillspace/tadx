package list_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	page  datasourcelist.Page
	input datasourcelist.PageRequest
	calls int
}

func (r *reader) ListDatasources(_ context.Context, input datasourcelist.PageRequest) (datasourcelist.Page, error) {
	r.calls++
	r.input = input
	return r.page, nil
}

func TestOutputGolden(t *testing.T) {
	hasExtracts := true
	isCertified := false
	size := int64(42)
	output := datasourcelist.Output{
		Status: "listed", Environment: "dev", Site: "sandbox",
		Page: datasourcelist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "next-page"},
		Datasources: []datasourcelist.Datasource{{
			LUID: "datasource-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops",
			Type: "hyper", ContentURL: "sales", UpdatedAt: "2026-09-01T00:00:00Z",
			Description: "Sales data", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z",
			Size: &size, HasExtracts: &hasExtracts, IsCertified: &isCertified, Tags: []string{"daily"},
		}},
		RequestID: "request-1", Help: []string{"tadx content datasource inspect --id <datasource-luid>"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
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

func TestActionListsOneBoundedDatasourcePage(t *testing.T) {
	r := &reader{page: datasourcelist.Page{Number: 1, Size: 2, Total: 3, RequestID: "request-1", Datasources: []datasourcelist.Datasource{
		{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops"},
		{LUID: "ds-2", Name: "Finance", ProjectLUID: "p-2", ProjectName: "Finance"},
	}}}
	input := datasourcelist.Input{Environment: "dev", Site: "site", Limit: 2, Name: "Sales", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01T00:00:00Z", UpdatedBefore: "2026-09-01T00:00:00Z"}
	output, err := datasourcelist.New(r).Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Page.Returned != 2 || output.Page.NextCursor == "" || output.RequestID != "request-1" {
		t.Fatalf("output = %#v", output)
	}
	want := datasourcelist.PageRequest{PageNumber: 1, PageSize: 2, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore}
	if r.input != want {
		t.Fatalf("reader input = %#v", r.input)
	}
}

func TestActionCursorIsBoundToEveryDatasourceFilter(t *testing.T) {
	input := datasourcelist.Input{Environment: "dev", Site: "site-a", Limit: 1, Name: "Private", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	first, err := datasourcelist.New(&reader{page: datasourcelist.Page{Number: 1, Size: 1, Total: 2, Datasources: []datasourcelist.Datasource{{LUID: "ds-1"}}}}).Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(first.Page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(decoded), input.Name) || strings.Contains(string(decoded), input.OwnerName) {
		t.Fatalf("cursor exposes raw filters: %s", decoded)
	}
	tests := []struct {
		name   string
		mutate func(*datasourcelist.Input)
	}{
		{name: "environment", mutate: func(v *datasourcelist.Input) { v.Environment = "prod" }},
		{name: "site", mutate: func(v *datasourcelist.Input) { v.Site = "site-b" }},
		{name: "name", mutate: func(v *datasourcelist.Input) { v.Name = "Other" }},
		{name: "owner", mutate: func(v *datasourcelist.Input) { v.OwnerName = "Other" }},
		{name: "project LUID", mutate: func(v *datasourcelist.Input) { v.ProjectLUID = "p-2" }},
		{name: "project", mutate: func(v *datasourcelist.Input) { v.ProjectName = "Other" }},
		{name: "type", mutate: func(v *datasourcelist.Input) { v.Type = "sqlserver" }},
		{name: "tag", mutate: func(v *datasourcelist.Input) { v.Tag = "weekly" }},
		{name: "updated after", mutate: func(v *datasourcelist.Input) { v.UpdatedAfter = "2026-02-01" }},
		{name: "updated before", mutate: func(v *datasourcelist.Input) { v.UpdatedBefore = "2026-08-01" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			continued := input
			continued.Cursor = first.Page.NextCursor
			test.mutate(&continued)
			r := &reader{}
			_, err := datasourcelist.New(r).Execute(context.Background(), continued)
			if err == nil || err.Error() != "datasource continuation cursor does not match the current filters" || r.calls != 0 {
				t.Fatalf("error = %v, calls = %d", err, r.calls)
			}
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
				t.Fatalf("error kind = %#v", structured)
			}
		})
	}
}

func TestActionRejectsCursorWithAbsurdPageNumber(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"p":999999999999,"s":25,"f":"x"}`))
	r := &reader{}
	_, err := datasourcelist.New(r).Execute(context.Background(), datasourcelist.Input{Environment: "dev", Site: "site", Cursor: cursor})
	if err == nil || err.Error() != "invalid datasource continuation cursor" || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func TestActionRejectsOutOfRangeLimitAsUsage(t *testing.T) {
	r := &reader{}
	_, err := datasourcelist.New(r).Execute(context.Background(), datasourcelist.Input{Environment: "dev", Site: "site", Limit: 10001})
	if err == nil || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func TestFullDatasourceListBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	output := datasourcelist.Output{Datasources: []datasourcelist.Datasource{{LUID: "ds-1", Tags: tags}}}
	full := output.FullOutput().(datasourcelist.FullResult)
	if len(full.Datasources[0].Tags) != 50 || full.Datasources[0].TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}
