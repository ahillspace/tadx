package project_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/identity"
)

type ancestryResolver struct {
	phases []map[string]projectops.MoveProject
	phase  int
}

func (r *ancestryResolver) BeginProjectResolution(ctx context.Context) context.Context {
	r.phase++
	return ctx
}
func (r *ancestryResolver) ResolveProject(_ context.Context, selector identity.Selector) (projectops.MoveProject, error) {
	item, found := r.phases[min(r.phase-1, len(r.phases)-1)][string(selector.LUID)]
	if !found {
		return projectops.MoveProject{}, fmt.Errorf("project %s is absent", selector.LUID)
	}
	return item, nil
}
func (*ancestryResolver) FindProjectCollisions(context.Context, string, string) ([]projectops.MoveProject, error) {
	return nil, nil
}

func TestProjectMoveRejectsInvalidAuthoritativeAncestry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		parent  projectops.MoveProject
		other   projectops.MoveProject
		message string
	}{
		{"descendant-with-unrelated-display-path", projectops.MoveProject{LUID: "destination", ParentLUID: "source", Path: "Unrelated"}, projectops.MoveProject{}, "descendant"},
		{"missing-ancestor", projectops.MoveProject{LUID: "destination", ParentLUID: "missing"}, projectops.MoveProject{}, "ancestry resolution"},
		{"preexisting-cycle", projectops.MoveProject{LUID: "destination", ParentLUID: "ancestor"}, projectops.MoveProject{LUID: "ancestor", ParentLUID: "destination"}, "contains a cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := map[string]projectops.MoveProject{"source": {LUID: "source", Name: "Source"}, "destination": tc.parent}
			if tc.other.LUID != "" {
				items[tc.other.LUID] = tc.other
			}
			resolver, writer := &ancestryResolver{phases: []map[string]projectops.MoveProject{items}}, &mover{}
			_, err := projectops.NewMove(resolver, writer).Execute(t.Context(), projectops.MoveInput{Environment: "fixture", Site: "site", ProjectSelector: identity.Selector{LUID: "source"}, ParentSelector: identity.Selector{LUID: "destination"}}, false)
			if err == nil || !strings.Contains(err.Error(), tc.message) || writer.calls != 0 {
				t.Fatalf("error=%v writes=%d", err, writer.calls)
			}
		})
	}
}

func TestProjectMoveRechecksAncestorChainBeforeWrite(t *testing.T) {
	before := map[string]projectops.MoveProject{"source": {LUID: "source", Name: "Source"}, "destination": {LUID: "destination", ParentLUID: "ancestor"}, "ancestor": {LUID: "ancestor"}}
	after := map[string]projectops.MoveProject{"source": before["source"], "destination": before["destination"], "ancestor": {LUID: "ancestor", ParentLUID: "source"}}
	resolver, writer := &ancestryResolver{phases: []map[string]projectops.MoveProject{before, after}}, &mover{}
	_, err := projectops.NewMove(resolver, writer).Execute(t.Context(), projectops.MoveInput{Environment: "fixture", Site: "site", ProjectSelector: identity.Selector{LUID: "source"}, ParentSelector: identity.Selector{LUID: "destination"}}, false)
	if err == nil || !strings.Contains(err.Error(), "descendant") || writer.calls != 0 || resolver.phase != 2 {
		t.Fatalf("error=%v writes=%d phases=%d", err, writer.calls, resolver.phase)
	}
}
