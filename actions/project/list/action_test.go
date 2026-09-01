package list_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	projectlist "github.com/ahillspace/tadx/actions/project/list"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	page  projectlist.Page
	input projectlist.PageRequest
	calls int
}

func TestOutputGolden(t *testing.T) {
	topLevel := false
	count := 3
	output := projectlist.Output{
		Status: "listed", Environment: "dev", Site: "sandbox",
		Page: projectlist.OutputPage{Returned: 1, Total: 3, Limit: 1, NextCursor: "next-page"},
		Projects: []projectlist.Project{{
			LUID: "project-1", Name: "Ops", ParentLUID: "project-root", Description: "Operations",
			OwnerLUID: "user-1", TopLevel: &topLevel, ProjectCount: &count,
		}},
		RequestID: "request-1", Help: []string{"tadx content project get --id <project-luid>"},
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
	gotText := strings.ReplaceAll(buffer.String(), "\r\n", "\n")
	wantText := strings.ReplaceAll(string(want), "\r\n", "\n")
	gotText = strings.TrimSuffix(gotText, "\n")
	wantText = strings.TrimSuffix(wantText, "\n")
	if gotText != wantText {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, wantText, gotText)
	}
}

func (r *reader) ListProjects(_ context.Context, input projectlist.PageRequest) (projectlist.Page, error) {
	r.calls++
	r.input = input
	return r.page, nil
}

func TestActionListsBoundedProjectPage(t *testing.T) {
	r := &reader{page: projectlist.Page{Number: 1, Size: 2, Total: 3, Projects: []projectlist.Project{
		{LUID: "p-1", Name: "Department"},
		{LUID: "p-2", Name: "Ops", ParentLUID: "p-1", Description: "Operations", OwnerLUID: "u-1"},
	}}}
	output, err := projectlist.New(r).Execute(context.Background(), projectlist.Input{Environment: "dev", Site: "site", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(projectlist.CompactResult)
	if compact.Page.Returned != 2 || compact.Page.Total != 3 || compact.Page.NextCursor == "" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	if !reflect.DeepEqual(r.input, projectlist.PageRequest{PageNumber: 1, PageSize: 2}) {
		t.Fatalf("reader input = %#v", r.input)
	}
	full := output.FullOutput().(projectlist.FullResult)
	if full.Projects[1].Description != "Operations" || full.Projects[1].OwnerLUID != "u-1" {
		t.Fatalf("full = %#v", full)
	}
}

func TestActionCursorContinuesWithoutChangingLimit(t *testing.T) {
	firstReader := &reader{page: projectlist.Page{Number: 1, Size: 25, Total: 26, Projects: make([]projectlist.Project, 25)}}
	first, err := projectlist.New(firstReader).Execute(context.Background(), projectlist.Input{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	r := &reader{page: projectlist.Page{Number: 2, Size: 25, Total: 26, Projects: []projectlist.Project{{LUID: "p-26", Name: "Last"}}}}
	_, err = projectlist.New(r).Execute(context.Background(), projectlist.Input{Environment: "dev", Cursor: first.Page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.PageNumber != 2 || r.input.PageSize != 25 {
		t.Fatalf("input = %#v", r.input)
	}
}

func TestActionCursorIsBoundToProjectFilters(t *testing.T) {
	topLevel := true
	firstReader := &reader{page: projectlist.Page{Number: 1, Size: 2, Total: 3, Projects: make([]projectlist.Project, 2)}}
	first, err := projectlist.New(firstReader).Execute(context.Background(), projectlist.Input{
		Environment: "dev",
		Limit:       2,
		Name:        "Private project name",
		ParentLUID:  "parent-1",
		OwnerName:   "Private owner name",
		TopLevel:    &topLevel,
	})
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(first.Page.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(decoded), "Private project name") || strings.Contains(string(decoded), "Private owner name") {
		t.Fatalf("cursor exposes raw filters: %s", decoded)
	}

	falseValue := false
	tests := []struct {
		name   string
		mutate func(*projectlist.Input)
	}{
		{name: "name", mutate: func(input *projectlist.Input) { input.Name = "Other" }},
		{name: "parent", mutate: func(input *projectlist.Input) { input.ParentLUID = "parent-2" }},
		{name: "owner", mutate: func(input *projectlist.Input) { input.OwnerName = "Other" }},
		{name: "top level", mutate: func(input *projectlist.Input) { input.TopLevel = &falseValue }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := projectlist.Input{Environment: "dev", Cursor: first.Page.NextCursor, Limit: 2, Name: "Private project name", ParentLUID: "parent-1", OwnerName: "Private owner name", TopLevel: &topLevel}
			test.mutate(&input)
			continuationReader := &reader{}
			_, err := projectlist.New(continuationReader).Execute(context.Background(), input)
			if err == nil || err.Error() != "project continuation cursor does not match the current filters" {
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

func TestActionCursorIsBoundToResolvedProjectEnvironment(t *testing.T) {
	firstReader := &reader{page: projectlist.Page{Number: 1, Size: 1, Total: 2, Projects: []projectlist.Project{{LUID: "p-1"}}}}
	first, err := projectlist.New(firstReader).Execute(context.Background(), projectlist.Input{Environment: "dev", Site: "site-a", Limit: 1, Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []projectlist.Input{
		{Environment: "production", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Ops"},
		{Environment: "dev", Site: "site-b", Cursor: first.Page.NextCursor, Name: "Ops"},
	} {
		continuationReader := &reader{}
		_, err = projectlist.New(continuationReader).Execute(context.Background(), input)
		if err == nil || err.Error() != "project continuation cursor does not match the current filters" {
			t.Fatalf("error = %v", err)
		}
		if continuationReader.calls != 0 {
			t.Fatalf("reader calls = %d", continuationReader.calls)
		}
	}
}

func TestActionRejectsProjectCursorVersionAndLimitMismatch(t *testing.T) {
	legacy := base64.RawURLEncoding.EncodeToString([]byte(`{"p":2,"s":2}`))
	_, err := projectlist.New(&reader{}).Execute(context.Background(), projectlist.Input{Cursor: legacy})
	assertUsageError(t, err, "invalid project continuation cursor")

	first, err := projectlist.New(&reader{page: projectlist.Page{Number: 1, Size: 2, Total: 3, Projects: make([]projectlist.Project, 2)}}).Execute(context.Background(), projectlist.Input{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	continuationReader := &reader{}
	_, err = projectlist.New(continuationReader).Execute(context.Background(), projectlist.Input{Cursor: first.Page.NextCursor, Limit: 3})
	assertUsageError(t, err, "project list limit must match the continuation cursor")
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

func TestFullProjectOutputIsBoundedToCurrentPage(t *testing.T) {
	projects := make([]projectlist.Project, 100)
	for index := range projects {
		projects[index] = projectlist.Project{LUID: fmt.Sprintf("p-%03d", index), Name: "Project"}
	}
	r := &reader{page: projectlist.Page{Number: 1, Size: 100, Total: 100, Projects: projects}}
	output, err := projectlist.New(r).Execute(context.Background(), projectlist.Input{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(output.FullOutput().(projectlist.FullResult).Projects); got != 100 {
		t.Fatalf("full projects = %d", got)
	}
}
