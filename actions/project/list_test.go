package project_test

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

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	page  projectops.Page
	input projectops.PageRequest
	calls int
}

func TestProjectListOutputGolden(t *testing.T) {
	topLevel := false
	count := 3
	output := projectops.ListOutput{
		Status: "listed", Environment: "dev", Site: "sandbox",
		Page: projectops.OutputPage{Returned: 1, Total: 3, Limit: 1, NextCursor: "next-page"},
		Projects: []projectops.ListProject{{
			LUID: "project-1", Name: "Ops", ParentLUID: "project-root", Description: "Operations",
			OwnerLUID: "user-1", TopLevel: &topLevel, ProjectCount: &count,
		}},
		RequestID: "request-1", Help: []string{"tadx content project inspect --project-id <project-luid>"},
	}
	assertListGolden(t, "compact.toon", output, false)
	assertListGolden(t, "full.toon", output, true)
}

func TestCompactDiscoveryIncludesPermissionControlMetadata(t *testing.T) {
	output := projectops.ListOutput{Projects: []projectops.ListProject{{LUID: "child", Name: "Child", ParentLUID: "root", ContentPermissions: "LockedToProject", ControllingPermissionsProjectID: "root"}}}
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, output, render.Options{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buffer.String(), "content_permissions") || !strings.Contains(buffer.String(), "controlling_permissions_project_luid") {
		t.Fatalf("compact discovery omitted permissions: %s", buffer.String())
	}
}

func assertListGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/list", name))
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

func (r *reader) ListProjects(_ context.Context, input projectops.PageRequest) (projectops.Page, error) {
	r.calls++
	r.input = input
	return r.page, nil
}

func TestActionListsBoundedProjectPage(t *testing.T) {
	r := &reader{page: projectops.Page{Number: 1, Size: 2, Total: 3, Projects: []projectops.ListProject{
		{LUID: "p-1", Name: "Department"},
		{LUID: "p-2", Name: "Ops", ParentLUID: "p-1", Description: "Operations", OwnerLUID: "u-1"},
	}}}
	output, err := projectops.NewList(r).Execute(context.Background(), projectops.ListInput{Environment: "dev", Site: "site", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(projectops.ListCompactResult)
	if compact.Page.Returned != 2 || compact.Page.Total != 3 || compact.Page.NextCursor == "" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	if !reflect.DeepEqual(r.input, projectops.PageRequest{PageNumber: 1, PageSize: 2}) {
		t.Fatalf("reader input = %#v", r.input)
	}
	full := output.FullOutput().(projectops.ListFullResult)
	if full.Projects[1].Description != "Operations" || full.Projects[1].OwnerLUID != "u-1" {
		t.Fatalf("full = %#v", full)
	}
}

func TestActionCursorContinuesWithoutChangingLimit(t *testing.T) {
	firstReader := &reader{page: projectops.Page{Number: 1, Size: 25, Total: 26, Projects: make([]projectops.ListProject, 25)}}
	first, err := projectops.NewList(firstReader).Execute(context.Background(), projectops.ListInput{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	r := &reader{page: projectops.Page{Number: 2, Size: 25, Total: 26, Projects: []projectops.ListProject{{LUID: "p-26", Name: "Last"}}}}
	_, err = projectops.NewList(r).Execute(context.Background(), projectops.ListInput{Environment: "dev", Cursor: first.Page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if r.input.PageNumber != 2 || r.input.PageSize != 25 {
		t.Fatalf("input = %#v", r.input)
	}
}

func TestActionCursorIsBoundToProjectFilters(t *testing.T) {
	topLevel := true
	firstReader := &reader{page: projectops.Page{Number: 1, Size: 2, Total: 3, Projects: make([]projectops.ListProject, 2)}}
	first, err := projectops.NewList(firstReader).Execute(context.Background(), projectops.ListInput{
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
		mutate func(*projectops.ListInput)
	}{
		{name: "name", mutate: func(input *projectops.ListInput) { input.Name = "Other" }},
		{name: "parent", mutate: func(input *projectops.ListInput) { input.ParentLUID = "parent-2" }},
		{name: "owner", mutate: func(input *projectops.ListInput) { input.OwnerName = "Other" }},
		{name: "top level", mutate: func(input *projectops.ListInput) { input.TopLevel = &falseValue }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := projectops.ListInput{Environment: "dev", Cursor: first.Page.NextCursor, Limit: 2, Name: "Private project name", ParentLUID: "parent-1", OwnerName: "Private owner name", TopLevel: &topLevel}
			test.mutate(&input)
			continuationReader := &reader{}
			_, err := projectops.NewList(continuationReader).Execute(context.Background(), input)
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
	firstReader := &reader{page: projectops.Page{Number: 1, Size: 1, Total: 2, Projects: []projectops.ListProject{{LUID: "p-1"}}}}
	first, err := projectops.NewList(firstReader).Execute(context.Background(), projectops.ListInput{Environment: "dev", Site: "site-a", Limit: 1, Name: "Ops"})
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []projectops.ListInput{
		{Environment: "production", Site: "site-a", Cursor: first.Page.NextCursor, Name: "Ops"},
		{Environment: "dev", Site: "site-b", Cursor: first.Page.NextCursor, Name: "Ops"},
	} {
		continuationReader := &reader{}
		_, err = projectops.NewList(continuationReader).Execute(context.Background(), input)
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
	_, err := projectops.NewList(&reader{}).Execute(context.Background(), projectops.ListInput{Cursor: legacy})
	assertUsageError(t, err, "invalid project continuation cursor")

	first, err := projectops.NewList(&reader{page: projectops.Page{Number: 1, Size: 2, Total: 3, Projects: make([]projectops.ListProject, 2)}}).Execute(context.Background(), projectops.ListInput{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	continuationReader := &reader{}
	_, err = projectops.NewList(continuationReader).Execute(context.Background(), projectops.ListInput{Cursor: first.Page.NextCursor, Limit: 3})
	assertUsageError(t, err, "project list limit must match the continuation cursor")
	if continuationReader.calls != 0 {
		t.Fatalf("reader calls = %d", continuationReader.calls)
	}
}

func TestActionRejectsInvalidProjectPagingBeforeReading(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   projectops.ListInput
		message string
	}{
		{name: "malformed cursor", input: projectops.ListInput{Cursor: "not-base64!"}, message: "invalid project continuation cursor"},
		{name: "all with limit", input: projectops.ListInput{All: true, Limit: 1}, message: "--all cannot be combined with --limit or --cursor"},
		{name: "all with cursor", input: projectops.ListInput{All: true, Cursor: "not-base64!"}, message: "--all cannot be combined with --limit or --cursor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &reader{}
			_, err := projectops.NewList(r).Execute(t.Context(), test.input)
			assertUsageError(t, err, test.message)
			if r.calls != 0 {
				t.Fatalf("reader calls = %d", r.calls)
			}
		})
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
	projects := make([]projectops.ListProject, 100)
	for index := range projects {
		projects[index] = projectops.ListProject{LUID: fmt.Sprintf("p-%03d", index), Name: "Project"}
	}
	r := &reader{page: projectops.Page{Number: 1, Size: 100, Total: 100, Projects: projects}}
	output, err := projectops.NewList(r).Execute(context.Background(), projectops.ListInput{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(output.FullOutput().(projectops.ListFullResult).Projects); got != 100 {
		t.Fatalf("full projects = %d", got)
	}
}
