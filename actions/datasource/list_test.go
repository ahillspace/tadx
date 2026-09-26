package datasource_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type listReader struct {
	page  datasourceops.ListPage
	input datasourceops.ListPageRequest
	calls int
}

func (r *listReader) ListDatasources(_ context.Context, input datasourceops.ListPageRequest) (datasourceops.ListPage, error) {
	r.calls++
	r.input = input
	return r.page, nil
}

func TestListOutputGolden(t *testing.T) {
	hasExtracts := true
	isCertified := false
	size := int64(42)
	output := datasourceops.ListOutput{
		Status: "listed", Environment: "dev", Site: "sandbox",
		Page: datasourceops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "next-page"},
		Datasources: []datasourceops.Record{{
			LUID: "datasource-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops",
			Type: "hyper", ContentURL: "sales", UpdatedAt: "2026-09-01T00:00:00Z",
			Description: "Sales data", OwnerLUID: "user-1", CreatedAt: "2026-08-01T00:00:00Z",
			Size: &size, HasExtracts: &hasExtracts, IsCertified: &isCertified, Tags: []string{"daily"},
		}},
		RequestID: "request-1", Help: []string{"tadx content datasource inspect --id <datasource-luid>"},
	}
	listAssertGolden(t, "compact.toon", output, false)
	listAssertGolden(t, "full.toon", output, true)
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

func TestListActionListsOneBoundedDatasourcePage(t *testing.T) {
	r := &listReader{page: datasourceops.ListPage{Number: 1, Size: 2, Total: 3, RequestID: "request-1", Datasources: []datasourceops.Record{
		{LUID: "ds-1", Name: "Sales", ProjectLUID: "p-1", ProjectName: "Ops"},
		{LUID: "ds-2", Name: "Finance", ProjectLUID: "p-2", ProjectName: "Finance"},
	}}}
	input := datasourceops.ListInput{Environment: "dev", Site: "site", Limit: 2, Name: "Sales", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01T00:00:00Z", UpdatedBefore: "2026-09-01T00:00:00Z"}
	output, err := datasourceops.List(context.Background(), r, input)
	if err != nil {
		t.Fatal(err)
	}
	if output.Page.Returned != 2 || output.Page.NextCursor == "" || output.RequestID != "request-1" {
		t.Fatalf("output = %#v", output)
	}
	want := datasourceops.ListPageRequest{PageNumber: 1, PageSize: 2, Name: input.Name, OwnerName: input.OwnerName, ProjectLUID: input.ProjectLUID, ProjectName: input.ProjectName, Type: input.Type, Tag: input.Tag, UpdatedAfter: input.UpdatedAfter, UpdatedBefore: input.UpdatedBefore}
	if r.input != want {
		t.Fatalf("reader input = %#v", r.input)
	}
}

func TestListActionCursorIsBoundToEveryDatasourceFilter(t *testing.T) {
	input := datasourceops.ListInput{Environment: "dev", Site: "site-a", Limit: 1, Name: "Private", OwnerName: "owner", ProjectLUID: "p-1", ProjectName: "Ops", Type: "hyper", Tag: "daily", UpdatedAfter: "2026-01-01", UpdatedBefore: "2026-09-01"}
	first, err := datasourceops.List(context.Background(), &listReader{page: datasourceops.ListPage{Number: 1, Size: 1, Total: 2, Datasources: []datasourceops.Record{{LUID: "ds-1"}}}}, input)
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
		mutate func(*datasourceops.ListInput)
	}{
		{name: "environment", mutate: func(v *datasourceops.ListInput) { v.Environment = "prod" }},
		{name: "site", mutate: func(v *datasourceops.ListInput) { v.Site = "site-b" }},
		{name: "name", mutate: func(v *datasourceops.ListInput) { v.Name = "Other" }},
		{name: "owner", mutate: func(v *datasourceops.ListInput) { v.OwnerName = "Other" }},
		{name: "project LUID", mutate: func(v *datasourceops.ListInput) { v.ProjectLUID = "p-2" }},
		{name: "project", mutate: func(v *datasourceops.ListInput) { v.ProjectName = "Other" }},
		{name: "type", mutate: func(v *datasourceops.ListInput) { v.Type = "sqlserver" }},
		{name: "tag", mutate: func(v *datasourceops.ListInput) { v.Tag = "weekly" }},
		{name: "updated after", mutate: func(v *datasourceops.ListInput) { v.UpdatedAfter = "2026-02-01" }},
		{name: "updated before", mutate: func(v *datasourceops.ListInput) { v.UpdatedBefore = "2026-08-01" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			continued := input
			continued.Cursor = first.Page.NextCursor
			test.mutate(&continued)
			r := &listReader{}
			_, err := datasourceops.List(context.Background(), r, continued)
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

func TestListActionRejectsCursorWithAbsurdPageNumber(t *testing.T) {
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"p":999999999999,"s":25,"f":"x"}`))
	r := &listReader{}
	_, err := datasourceops.List(context.Background(), r, datasourceops.ListInput{Environment: "dev", Site: "site", Cursor: cursor})
	if err == nil || err.Error() != "invalid datasource continuation cursor" || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func TestListActionRejectsOutOfRangeLimitAsUsage(t *testing.T) {
	r := &listReader{}
	_, err := datasourceops.List(context.Background(), r, datasourceops.ListInput{Environment: "dev", Site: "site", Limit: 10001})
	if err == nil || r.calls != 0 {
		t.Fatalf("error = %v, calls = %d", err, r.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error kind = %#v", structured)
	}
}

func TestListFullDatasourceListBoundsTags(t *testing.T) {
	tags := make([]string, 60)
	output := datasourceops.ListOutput{Datasources: []datasourceops.Record{{LUID: "ds-1", Tags: tags}}}
	full := output.FullOutput().(datasourceops.ListFullResult)
	if len(full.Datasources[0].Tags) != 50 || full.Datasources[0].TagsOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}
