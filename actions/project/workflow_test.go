package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	projectops "github.com/ahillspace/tadx/actions/project"
)

type workflowProvider struct {
	opens       int
	filterCalls int
	filterError error
}

func (p *workflowProvider) CacheTarget(string) (projectops.Target, error) {
	return projectops.Target{}, errors.New("unexpected cache target resolution")
}
func (p *workflowProvider) CachedList(projectops.Target) projectops.CachedListReader { return nil }
func (p *workflowProvider) CachedInspect(projectops.Target) projectops.CachedInspectResolver {
	return nil
}
func (p *workflowProvider) ListFilter(projectops.ListInput) (string, error) {
	p.filterCalls++
	return "", p.filterError
}
func (p *workflowProvider) Open(context.Context, string, string, string, bool) (projectops.LiveSession, error) {
	p.opens++
	return projectops.LiveSession{}, errors.New("unexpected target open")
}
func (p *workflowProvider) Now() time.Time { return time.Unix(0, 0) }

func TestProjectWorkflowRejectsInvalidInputBeforeOpeningTarget(t *testing.T) {
	tests := []struct {
		name string
		call func(*projectops.Service) error
	}{
		{"create", func(s *projectops.Service) error {
			_, err := s.CreateProject(t.Context(), projectops.CreateInput{Environment: "dev"}, false)
			return err
		}},
		{"update", func(s *projectops.Service) error {
			_, err := s.UpdateProject(t.Context(), projectops.UpdateInput{Environment: "dev"}, false)
			return err
		}},
		{"delete", func(s *projectops.Service) error {
			_, err := s.DeleteProject(t.Context(), projectops.DeleteInput{Environment: "dev"}, false)
			return err
		}},
		{"move", func(s *projectops.Service) error {
			_, err := s.MoveProject(t.Context(), projectops.MoveInput{Environment: "dev"}, false)
			return err
		}},
		{"inspect", func(s *projectops.Service) error {
			_, err := s.InspectProject(t.Context(), projectops.InspectInput{Environment: "dev"})
			return err
		}},
		{"list", func(s *projectops.Service) error {
			_, err := s.ListProjects(t.Context(), projectops.ListInput{Environment: "dev", All: true, Limit: 1})
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &workflowProvider{}
			if err := test.call(projectops.New(projectops.Ports{Provider: provider})); err == nil || provider.opens != 0 {
				t.Fatalf("error=%v target opens=%d", err, provider.opens)
			}
		})
	}
}

func TestProjectListFilterFailsBeforeOpeningTarget(t *testing.T) {
	provider := &workflowProvider{filterError: errors.New("invalid project filter")}
	_, err := projectops.New(projectops.Ports{Provider: provider}).ListProjects(t.Context(), projectops.ListInput{Environment: "dev", Name: "A&B"})
	if err == nil || provider.filterCalls != 1 || provider.opens != 0 {
		t.Fatalf("error=%v filter calls=%d target opens=%d", err, provider.filterCalls, provider.opens)
	}
}

func TestProjectCreateUsesResolvedTargetWithoutRepeatingCallerValidation(t *testing.T) {
	resolver := &createResolver{}
	creator := &createCreator{}
	service := newTestServiceForTarget(projectops.Ports{CreateResolver: resolver, Creator: creator}, projectops.Target{Environment: "canonical", Site: "exact-site"})
	out, err := service.CreateProject(t.Context(), projectops.CreateInput{Environment: "selected", Site: "stale-site", Name: "Operations"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Plan.Environment != "canonical" || out.Plan.Site != "exact-site" || creator.calls != 0 {
		t.Fatalf("resolved target or preview contract changed: plan=%+v writes=%d", out.Plan, creator.calls)
	}
}
