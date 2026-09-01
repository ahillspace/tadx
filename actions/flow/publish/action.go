package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

type ArtifactReader interface {
	ReadFlow(context.Context, string) (Artifact, error)
}
type Resolver interface {
	ResolveProject(context.Context, identity.Selector) (Project, error)
	FindFlows(context.Context, string, string) ([]Flow, error)
}
type PreparedPublish interface {
	Commit(context.Context) (Result, error)
}
type Publisher interface {
	Prepare(context.Context, PublishRequest) (PreparedPublish, error)
}
type Action struct {
	artifacts ArtifactReader
	resolver  Resolver
	publisher Publisher
}

func New(artifacts ArtifactReader, resolver Resolver, publisher Publisher) *Action {
	return &Action{artifacts: artifacts, resolver: resolver, publisher: publisher}
}
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error) {
	if a == nil || a.artifacts == nil || a.resolver == nil || a.publisher == nil {
		return Output{}, errors.New("flow publish dependencies are not configured")
	}
	plan, err := a.plan(ctx, input)
	if err != nil {
		return Output{}, err
	}
	output := Output{Plan: plan, Applied: false, Help: []string{"Add --apply to publish this exact plan."}}
	if !apply {
		return output, nil
	}
	prepared, err := a.publisher.Prepare(ctx, plan.request)
	if err != nil {
		return Output{}, err
	}
	current, err := a.artifacts.ReadFlow(ctx, input.ArtifactPath)
	if err != nil {
		return Output{}, err
	}
	if current.Fingerprint != plan.ArtifactFingerprint {
		return Output{}, errors.New("flow artifact changed after preview")
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		return Output{}, err
	}
	if project.LUID != plan.Target.ProjectLUID || project.Path != plan.Target.ProjectPath {
		return Output{}, errors.New("flow publish destination changed after preview")
	}
	existing, err := exactCollision(ctx, a.resolver, plan.FlowName, project.LUID, input.Overwrite)
	if err != nil {
		return Output{}, err
	}
	if existing != plan.Target.ExistingLUID {
		return Output{}, errors.New("flow publish collision changed after preview")
	}
	result, err := prepared.Commit(ctx)
	if err != nil {
		return Output{}, err
	}
	output.Applied = true
	output.Result = &result
	output.Help = []string{"tadx content flow get --id " + result.FlowLUID}
	return output, nil
}
func (a *Action) plan(ctx context.Context, input Input) (Plan, error) {
	if input.Environment == "" || input.Site == "" {
		return Plan{}, errors.New("flow publish requires an explicit resolved environment and site")
	}
	artifact, err := a.artifacts.ReadFlow(ctx, input.ArtifactPath)
	if err != nil {
		return Plan{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = artifact.Name
	}
	if name == "" {
		return Plan{}, errors.New("flow publish requires a flow name")
	}
	project, err := a.resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		return Plan{}, err
	}
	existing, err := exactCollision(ctx, a.resolver, name, project.LUID, input.Overwrite)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Mode: "preview", Operation: "flow.publish", ArtifactPath: artifact.Path, ArtifactFingerprint: artifact.Fingerprint, Filename: artifact.Filename, FlowName: name, Target: Target{Environment: input.Environment, Site: input.Site, ProjectLUID: project.LUID, ProjectPath: project.Path, ExistingLUID: existing}, Overwrite: input.Overwrite, Substeps: []string{"resolve exact destination", "check flow collision", "upload native flow", "revalidate destination and collision", "publish flow"}, request: PublishRequest{Name: name, ProjectLUID: project.LUID, Filename: artifact.Filename, ContentPath: artifact.PayloadPath, ContentSize: artifact.Size, ExpectedFingerprint: artifact.Fingerprint, Overwrite: input.Overwrite}, planned: true}, nil
}
func exactCollision(ctx context.Context, resolver Resolver, name, project string, overwrite bool) (string, error) {
	items, err := resolver.FindFlows(ctx, name, project)
	if err != nil {
		return "", err
	}
	if len(items) > 1 {
		return "", errors.New("multiple authoritative flows match the publish collision")
	}
	if len(items) == 0 {
		return "", nil
	}
	if items[0].LUID == "" || items[0].ProjectLUID != project {
		return "", errors.New("flow collision omitted authoritative destination identity")
	}
	if !overwrite {
		return "", fmt.Errorf("flow %q already exists in the destination; use --overwrite", name)
	}
	return items[0].LUID, nil
}
