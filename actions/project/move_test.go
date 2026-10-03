package project_test

import (
	"context"
	"fmt"
	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
	"testing"
)

type moveResolver struct {
	source, parent projectops.MoveProject
	matches        []projectops.MoveProject
}

func (r moveResolver) BeginProjectResolution(ctx context.Context) context.Context { return ctx }

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
	source string
}

func (m *mover) MoveProject(_ context.Context, source string, parent *string) (projectops.MoveResult, error) {
	m.calls++
	m.source = source
	m.parent = parent
	return projectops.MoveResult{Status: "succeeded", Project: projectops.MoveProject{LUID: "p-1", Name: "Child", Path: "New/Child", ParentLUID: *parent}}, nil
}
func TestMovePreviewsThenRevalidatesParent(t *testing.T) {
	r := moveResolver{source: projectops.MoveProject{LUID: "p-1", Name: "Child", Path: "Old/Child", ParentLUID: "old"}, parent: projectops.MoveProject{LUID: "new", Name: "New", Path: "New"}}
	m := &mover{}
	a := projectops.New(projectops.Ports{MoveResolver: r, Mover: m})
	in := projectops.MoveInput{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "new"}}
	if out, err := a.Move(context.Background(), in, true); err != nil || out.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v err=%v", out, err)
	}
	if out, err := a.Move(context.Background(), in, false); err != nil || out.Result == nil || m.calls != 1 || m.source != "p-1" || m.parent == nil || *m.parent != "new" {
		t.Fatalf("execute=%#v err=%v parent=%v", out, err, m.parent)
	}
}

type moveChangingResolver struct {
	phase, collisions int
	descendant        bool
}

func (r *moveChangingResolver) BeginProjectResolution(ctx context.Context) context.Context {
	r.phase++
	return ctx
}

func (r *moveChangingResolver) ResolveProject(_ context.Context, selector identity.Selector) (projectops.MoveProject, error) {
	switch string(selector.LUID) {
	case "source":
		return projectops.MoveProject{LUID: "source", Name: "Source", Path: "Source"}, nil
	case "destination":
		path := "Destination"
		if r.descendant && r.phase > 1 {
			path = "Source/Destination"
		}
		return projectops.MoveProject{LUID: "destination", Name: "Destination", Path: path}, nil
	default:
		return projectops.MoveProject{}, fmt.Errorf("unexpected selector: %#v", selector)
	}
}

func (r *moveChangingResolver) FindProjectCollisions(_ context.Context, name, parent string) ([]projectops.MoveProject, error) {
	r.collisions++
	if name != "Source" || parent != "destination" {
		return nil, fmt.Errorf("wrong collision scope: %q %q", name, parent)
	}
	if !r.descendant && r.phase > 1 {
		return []projectops.MoveProject{{LUID: "other", Name: "Source", ParentLUID: "destination"}}, nil
	}
	return nil, nil
}

func TestMoveRejectsFreshPhaseDestinationChangesBeforeWriting(t *testing.T) {
	for _, descendant := range []bool{false, true} {
		t.Run(fmt.Sprintf("descendant=%t", descendant), func(t *testing.T) {
			r := &moveChangingResolver{descendant: descendant}
			m := &mover{}
			_, err := projectops.New(projectops.Ports{MoveResolver: r, Mover: m}).Move(t.Context(), projectops.MoveInput{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "source"}, ParentSelector: identity.Selector{LUID: "destination"}}, false)
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
	r := moveResolver{source: projectops.MoveProject{LUID: "p-1", Name: "Root", Path: "Root"}, parent: projectops.MoveProject{LUID: "child", Name: "Child", Path: "Root/Child"}}
	if _, err := projectops.New(projectops.Ports{MoveResolver: r, Mover: &mover{}}).Move(context.Background(), projectops.MoveInput{Environment: "dev", Site: "site", ProjectSelector: identity.Selector{LUID: "p-1"}, ParentSelector: identity.Selector{LUID: "child"}}, false); err == nil {
		t.Fatal("expected cycle error")
	}
}
