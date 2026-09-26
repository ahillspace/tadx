package workbook_test

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type moveResolver struct {
	workbook workbookops.Record
	project  workbookops.Project
	matches  []workbookops.Record
	calls    int
}

func (r *moveResolver) ResolveWorkbook(context.Context, identity.Selector) (workbookops.Record, error) {
	r.calls++
	return r.workbook, nil
}
func (r *moveResolver) ResolveProject(context.Context, identity.Selector) (workbookops.Project, error) {
	return r.project, nil
}
func (r *moveResolver) FindWorkbooks(context.Context, string, string) ([]workbookops.Record, error) {
	return r.matches, nil
}

type moveMover struct{ calls int }

func (m *moveMover) MoveWorkbook(context.Context, string, string) (workbookops.MoveResult, error) {
	m.calls++
	return workbookops.MoveResult{Status: "succeeded", WorkbookLUID: "wb-1", ProjectLUID: "p-2"}, nil
}

func TestMovePreviewsThenRevalidatesAndMoves(t *testing.T) {
	r := &moveResolver{workbook: workbookops.Record{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1", OwnerLUID: "u-1"}, project: workbookops.Project{LUID: "p-2", Path: "New"}}
	m := &moveMover{}
	aResolver, aMover := r, m
	in := workbookops.MoveInput{Environment: "dev", Site: "site", WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}
	if out, err := workbookops.Move(context.Background(), aResolver, aMover, in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v calls=%d", out, err, m.calls)
	}
	if out, err := workbookops.Move(context.Background(), aResolver, aMover, in, false); err != nil || out.Result == nil || m.calls != 1 {
		t.Fatalf("execute=%#v err=%v calls=%d", out, err, m.calls)
	}
}

func TestMoveRejectsDestinationCollision(t *testing.T) {
	r := &moveResolver{workbook: workbookops.Record{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1"}, project: workbookops.Project{LUID: "p-2"}, matches: []workbookops.Record{{LUID: "wb-2", Name: "Sales", ProjectLUID: "p-2"}}}
	_, err := workbookops.Move(context.Background(), r, &moveMover{}, workbookops.MoveInput{Environment: "dev", Site: "site", WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, false)
	if err == nil {
		t.Fatal("expected collision error")
	}
}

func TestMoveDefaultSiteRequiresResolvedTarget(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment string
		resolved    bool
		wantError   bool
	}{
		{"unresolved", "dev", false, true},
		{"resolved default", "dev", true, false},
		{"missing environment", "", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &moveResolver{workbook: workbookops.Record{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1"}, project: workbookops.Project{LUID: "p-2", Path: "New"}}
			in := workbookops.MoveInput{Environment: test.environment, TargetResolved: test.resolved, WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}
			out, err := workbookops.Move(context.Background(), r, &moveMover{}, in, true)
			if (err != nil) != test.wantError {
				t.Fatalf("output=%#v err=%v", out, err)
			}
			if test.wantError && r.calls != 0 {
				t.Fatalf("unresolved target reached Tableau: %d reads", r.calls)
			}
		})
	}
}
