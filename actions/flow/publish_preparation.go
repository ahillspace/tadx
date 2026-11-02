package flow

import "github.com/ahillspace/tadx/internal/value"

func (p PublishPlan) Destination() value.PublishDestination {
	return value.PublishDestination{Environment: p.Target.Environment, Site: p.Target.Site, ProjectLUID: p.Target.ProjectLUID, Name: p.FlowName, ExistingLUID: p.Target.ExistingLUID}
}

func (input PublishInput) ExpectDestination(expected *value.PublishDestination) PublishInput {
	if expected == nil {
		return input
	}
	input.Environment, input.Site, input.Name = expected.Environment, expected.Site, expected.Name
	input.SetProjectSelector(expected.ProjectLUID, "")
	input.ExpectedDestination = new(*expected)
	return input
}

func (p PublishPlan) PrepareInput(input PublishInput) PublishInput {
	input = input.ExpectDestination(new(p.Destination()))
	if p.Workspace != "" {
		input.Workspace = p.Workspace
	}
	return input
}
