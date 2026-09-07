package move_test

import (
	"context"
	"testing"

	workbookmove "github.com/ahillspace/tadx/actions/workbook/move"
	"github.com/ahillspace/tadx/internal/identity"
)

type resolver struct {
	workbook workbookmove.Workbook
	project  workbookmove.Project
	matches  []workbookmove.Workbook
	calls    int
}

func (r *resolver) ResolveWorkbook(context.Context, identity.Selector) (workbookmove.Workbook, error) {
	r.calls++
	return r.workbook, nil
}
func (r *resolver) ResolveProject(context.Context, identity.Selector) (workbookmove.Project, error) {
	return r.project, nil
}
func (r *resolver) FindWorkbooks(context.Context, string, string) ([]workbookmove.Workbook, error) {
	return r.matches, nil
}

type mover struct{ calls int }

func (m *mover) MoveWorkbook(context.Context, string, string) (workbookmove.Result, error) {
	m.calls++
	return workbookmove.Result{Status: "succeeded", WorkbookLUID: "wb-1", ProjectLUID: "p-2"}, nil
}

func TestMovePreviewsThenRevalidatesAndMoves(t *testing.T) {
	r := &resolver{workbook: workbookmove.Workbook{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1", OwnerLUID: "u-1"}, project: workbookmove.Project{LUID: "p-2", Path: "New"}}
	m := &mover{}
	a := workbookmove.New(r, m)
	in := workbookmove.Input{Environment: "dev", Site: "site", WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v calls=%d", out, err, m.calls)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || m.calls != 1 {
		t.Fatalf("execute=%#v err=%v calls=%d", out, err, m.calls)
	}
}

func TestMoveRejectsDestinationCollision(t *testing.T) {
	r := &resolver{workbook: workbookmove.Workbook{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1"}, project: workbookmove.Project{LUID: "p-2"}, matches: []workbookmove.Workbook{{LUID: "wb-2", Name: "Sales", ProjectLUID: "p-2"}}}
	_, err := workbookmove.New(r, &mover{}).Execute(context.Background(), workbookmove.Input{Environment: "dev", Site: "site", WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, false)
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
			r := &resolver{workbook: workbookmove.Workbook{LUID: "wb-1", Name: "Sales", ProjectLUID: "p-1"}, project: workbookmove.Project{LUID: "p-2", Path: "New"}}
			in := workbookmove.Input{Environment: test.environment, TargetResolved: test.resolved, WorkbookSelector: identity.Selector{LUID: "wb-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}
			out, err := workbookmove.New(r, &mover{}).Execute(context.Background(), in, true)
			if (err != nil) != test.wantError {
				t.Fatalf("output=%#v err=%v", out, err)
			}
			if test.wantError && r.calls != 0 {
				t.Fatalf("unresolved target reached Tableau: %d reads", r.calls)
			}
		})
	}
}
