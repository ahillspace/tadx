package project_test

import (
	"context"
	"testing"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/identity"
)

type moveResolver struct {
	source, parent projectops.MoveProject
	matches        []projectops.MoveProject
}

func (r moveResolver) ResolveProject(_ context.Context, s identity.Selector) (projectops.MoveProject, error) {
	if string(s.LUID) == r.parent.LUID {
		return r.parent, nil
	}
	return r.source, nil
}
func (r moveResolver) FindProjectCollisions(context.Context, string, string) ([]projectops.MoveProject, error) {
	return r.matches, nil
}

type mover struct {
	calls  int
	parent *string
}

func (m *mover) MoveProject(_ context.Context, _ string, parent *string) (projectops.MoveResult, error) {
	m.calls++
	m.parent = parent
	return projectops.MoveResult{Status: "succeeded", Project: projectops.MoveProject{LUID: "p-1", Name: "Child", Path: "New/Child", ParentLUID: *parent}}, nil
}
func TestMovePreviewsThenRevalidatesParent(t *testing.T) {
	r := moveResolver{source: projectops.MoveProject{LUID: "p-1", Name: "Child", Path: "Old/Child", ParentLUID: "old"}, parent: projectops.MoveProject{LUID: "new", Name: "New", Path: "New"}}
	m := &mover{}
	a := projectops.NewMove(r, m)
	in := projectops.MoveInput{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "new"}}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || m.calls != 1 || m.parent == nil || *m.parent != "new" {
		t.Fatalf("execute=%#v err=%v parent=%v", out, err, m.parent)
	}
}
func TestMoveRejectsDescendantParent(t *testing.T) {
	r := moveResolver{source: projectops.MoveProject{LUID: "p-1", Name: "Root", Path: "Root"}, parent: projectops.MoveProject{LUID: "child", Name: "Child", Path: "Root/Child"}}
	if _, err := projectops.NewMove(r, &mover{}).Execute(context.Background(), projectops.MoveInput{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "child"}}, false); err == nil {
		t.Fatal("expected cycle error")
	}
}
