package batchinput

import "github.com/ahillspace/tadx/internal/identity"

// NamedTarget identifies a case-insensitive sibling slot without changing the
// name retained by the action for execution and fresh-state checks.
func NamedTarget(environment, site, kind, collection, name string) Target {
	return Target{Environment: environment, Site: site, ResourceKind: kind, Collection: collection, Name: identity.CollisionNameKey(name)}
}

// PublicationTargets retains both the named destination slot and the existing
// authoritative resource, so aliases and changes between row observations
// cannot admit two writes to either owned state.
func PublicationTargets(kind string, destination PublishDestination) []Target {
	targets := []Target{NamedTarget(destination.Environment, destination.Site, kind, destination.ProjectLUID, destination.Name)}
	if destination.ExistingLUID != "" {
		targets = append(targets, Target{Environment: destination.Environment, Site: destination.Site, ResourceKind: kind, ResourceID: destination.ExistingLUID})
	}
	return targets
}
