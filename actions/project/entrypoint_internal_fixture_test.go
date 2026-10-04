package project

import (
	"context"
	"time"

	"github.com/ahillspace/tadx/internal/readsource"
)

type internalTestProvider struct{ ports Ports }

func newInternalTestService(ports Ports) *Service {
	return New(Ports{Provider: internalTestProvider{ports: ports}})
}
func (p internalTestProvider) CacheTarget(environment string) (Target, error) {
	return Target{Environment: environment}, nil
}
func (p internalTestProvider) CachedList(Target) CachedListReader {
	return internalCachedList{p.ports.ListReader}
}
func (p internalTestProvider) CachedInspect(Target) CachedInspectResolver {
	return internalCachedInspect{p.ports.InspectResolver}
}
func (p internalTestProvider) LegacyInventoryCursor(string) bool    { return false }
func (p internalTestProvider) ListFilter(ListInput) (string, error) { return "", nil }
func (p internalTestProvider) Open(_ context.Context, environment, site, _ string, _ bool) (LiveSession, error) {
	return LiveSession{Target: Target{Environment: environment, Site: site}, Ports: p.ports, Inventory: internalTestInventory{p.ports.ListReader}}, nil
}
func (p internalTestProvider) ValidateComplete(bool, *readsource.Metadata) error { return nil }
func (p internalTestProvider) Now() time.Time                                    { return time.Unix(0, 0) }

type internalCachedList struct{ ListReader }

func (internalCachedList) Source() *readsource.Metadata { return nil }

type internalCachedInspect struct{ InspectResolver }

func (internalCachedInspect) Source() *readsource.Metadata { return nil }

type internalTestInventory struct{ reader ListReader }

func (i internalTestInventory) CollectProjects(context.Context, string, time.Time) (CollectedList, error) {
	return CollectedList{Reader: i.reader}, nil
}
func (internalTestInventory) PublishProjectInspect(context.Context, InspectOutput, time.Time) {}
