package move_test

import (
	"context"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type resolver struct {
	source, parent projectmove.Project
	matches        []projectmove.Project
}

func (r resolver) ResolveProject(_ context.Context, s identity.Selector) (projectmove.Project, error) {
	if string(s.LUID) == r.parent.LUID {
		return r.parent, nil
	}
	return r.source, nil
}
func (r resolver) FindProjectCollisions(context.Context, string, string) ([]projectmove.Project, error) {
	return r.matches, nil
}

type mover struct {
	calls  int
	parent *string
}

func (m *mover) MoveProject(_ context.Context, _ string, parent *string) (projectmove.Result, error) {
	m.calls++
	m.parent = parent
	return projectmove.Result{Status: "succeeded", Project: projectmove.Project{LUID: "p-1", Name: "Child", Path: "New/Child", ParentLUID: *parent}}, nil
}
func TestMovePreviewsThenRevalidatesParent(t *testing.T) {
	r := resolver{source: projectmove.Project{LUID: "p-1", Name: "Child", Path: "Old/Child", ParentLUID: "old"}, parent: projectmove.Project{LUID: "new", Name: "New", Path: "New"}}
	m := &mover{}
	a := projectmove.New(r, m)
	in := projectmove.Input{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "new"}}
	if out, err := a.Execute(context.Background(), in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || m.calls != 1 || m.parent == nil || *m.parent != "new" {
		t.Fatalf("execute=%#v err=%v parent=%v", out, err, m.parent)
	}
}
func TestMoveRejectsDescendantParent(t *testing.T) {
	r := resolver{source: projectmove.Project{LUID: "p-1", Name: "Root", Path: "Root"}, parent: projectmove.Project{LUID: "child", Name: "Child", Path: "Root/Child"}}
	if _, err := projectmove.New(r, &mover{}).Execute(context.Background(), projectmove.Input{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "child"}}, false); err == nil {
		t.Fatal("expected cycle error")
	}
}
