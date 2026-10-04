package project_test

import (
	"context"
	"time"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/readsource"
)

type testProjectProvider struct {
	ports      projectops.Ports
	target     projectops.Target
	openTarget projectops.Target
}

func newTestService(ports projectops.Ports) *projectops.Service {
	return projectops.New(projectops.Ports{Provider: testProjectProvider{ports: ports}})
}

func newTestServiceForTarget(ports projectops.Ports, target projectops.Target) *projectops.Service {
	return projectops.New(projectops.Ports{Provider: testProjectProvider{ports: ports, target: target}})
}

func (p testProjectProvider) selected(environment, site string) projectops.Target {
	if p.target.Environment != "" {
		environment = p.target.Environment
	}
	if p.target.Site != "" {
		site = p.target.Site
	}
	return projectops.Target{Environment: environment, Site: site}
}
func (p testProjectProvider) CacheTarget(environment string) (projectops.Target, error) {
	return p.selected(environment, p.target.Site), nil
}
func (p testProjectProvider) CachedList(projectops.Target) projectops.CachedListReader {
	return testCachedList{p.ports.ListReader}
}
func (p testProjectProvider) CachedInspect(projectops.Target) projectops.CachedInspectResolver {
	return testCachedInspect{p.ports.InspectResolver}
}
func (p testProjectProvider) ListFilter(projectops.ListInput) (string, error) { return "", nil }
func (p testProjectProvider) Open(_ context.Context, environment, site, _ string, _ bool) (projectops.LiveSession, error) {
	target := p.selected(environment, site)
	if p.openTarget.Environment != "" {
		target.Environment = p.openTarget.Environment
	}
	if p.openTarget.Site != "" {
		target.Site = p.openTarget.Site
	}
	return projectops.LiveSession{Target: target, Ports: p.ports, Inventory: testInventory{p.ports.ListReader}}, nil
}
func (p testProjectProvider) Now() time.Time { return time.Unix(0, 0) }

type testCachedList struct{ projectops.ListReader }

func (testCachedList) Source() *readsource.Metadata { return nil }

type testCachedInspect struct{ projectops.InspectResolver }

func (testCachedInspect) Source() *readsource.Metadata { return nil }

type testInventory struct{ reader projectops.ListReader }

func (i testInventory) CollectProjects(context.Context, string, time.Time) (projectops.CollectedList, error) {
	return projectops.CollectedList{Reader: i.reader}, nil
}
func (testInventory) PublishProjectInspect(context.Context, projectops.InspectOutput, time.Time) {}
