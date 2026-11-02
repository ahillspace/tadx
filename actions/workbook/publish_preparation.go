package workbook

import (
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// Destination is the exact mutation state selected by this publication plan.
func (p PublishPlan) Destination() value.PublishDestination {
	return value.PublishDestination{Environment: p.Target.Environment, Site: p.Target.Site, ProjectLUID: p.Target.ProjectLUID, Name: p.WorkbookName, ExistingLUID: p.Target.ExistingLUID}
}

// ExpectDestination pins an accepted destination before fresh action planning.
func (input PublishInput) ExpectDestination(expected *value.PublishDestination) PublishInput {
	if expected == nil {
		return input
	}
	input.Environment, input.Site, input.Name = expected.Environment, expected.Site, expected.Name
	input.ProjectLUID, input.ProjectPath = expected.ProjectLUID, ""
	input.ProjectSelector = identity.Selector{LUID: identity.LUID(expected.ProjectLUID)}
	input.ExpectedDestination = new(*expected)
	return input
}

// PrepareInput retains this plan's target while actions continue to revalidate
// the source, authenticated scope, collision, and final upload before writing.
func (p PublishPlan) PrepareInput(input PublishInput) PublishInput {
	input = input.ExpectDestination(new(p.Destination()))
	if p.Workspace != "" {
		input.Workspace = p.Workspace
	}
	return input
}
