package move_test

import (
	"context"
	"fmt"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
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
	source string
}

func (m *mover) MoveProject(_ context.Context, source string, parent *string) (projectmove.Result, error) {
	m.calls++
	m.source = source
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
	if out, err := a.Execute(context.Background(), in, false); err != nil || out.Result == nil || m.calls != 1 || m.source != "p-1" || m.parent == nil || *m.parent != "new" {
		t.Fatalf("execute=%#v err=%v parent=%v", out, err, m.parent)
	}
}

type changingResolver struct {
	phase, collisions int
	descendant        bool
}

func (r *changingResolver) BeginProjectResolution(ctx context.Context) context.Context {
	r.phase++
	return ctx
}

func (r *changingResolver) ResolveProject(_ context.Context, selector identity.Selector) (projectmove.Project, error) {
	switch string(selector.LUID) {
	case "source":
		return projectmove.Project{LUID: "source", Name: "Source", Path: "Source"}, nil
	case "destination":
		path := "Destination"
		if r.descendant && r.phase > 1 {
			path = "Source/Destination"
		}
		return projectmove.Project{LUID: "destination", Name: "Destination", Path: path}, nil
	default:
		return projectmove.Project{}, fmt.Errorf("unexpected selector: %#v", selector)
	}
}

func (r *changingResolver) FindProjectCollisions(_ context.Context, name, parent string) ([]projectmove.Project, error) {
	r.collisions++
	if name != "Source" || parent != "destination" {
		return nil, fmt.Errorf("wrong collision scope: %q %q", name, parent)
	}
	if !r.descendant && r.phase > 1 {
		return []projectmove.Project{{LUID: "other", Name: "Source", ParentLUID: "destination"}}, nil
	}
	return nil, nil
}

func TestMoveRejectsFreshPhaseDestinationChangesBeforeWriting(t *testing.T) {
	for _, descendant := range []bool{false, true} {
		t.Run(fmt.Sprintf("descendant=%t", descendant), func(t *testing.T) {
			r := &changingResolver{descendant: descendant}
			m := &mover{}
			_, err := projectmove.New(r, m).Execute(t.Context(), projectmove.Input{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "source"}, ParentSelector: identity.Selector{LUID: "destination"}}, false)
			want, reads := "already contains", 2
			if descendant {
				want, reads = "descendant", 1
			}
			if err == nil || !strings.Contains(err.Error(), want) || r.phase != 2 || r.collisions != reads || m.calls != 0 {
				t.Fatalf("error=%v phases=%d collision_reads=%d writes=%d", err, r.phase, r.collisions, m.calls)
			}
		})
	}
}
func TestMoveRejectsDescendantParent(t *testing.T) {
	r := resolver{source: projectmove.Project{LUID: "p-1", Name: "Root", Path: "Root"}, parent: projectmove.Project{LUID: "child", Name: "Child", Path: "Root/Child"}}
	if _, err := projectmove.New(r, &mover{}).Execute(context.Background(), projectmove.Input{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "child"}}, false); err == nil {
		t.Fatal("expected cycle error")
	}
}
