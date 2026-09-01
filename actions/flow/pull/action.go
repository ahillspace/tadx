package pull

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

type Reader interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
	DownloadFlow(context.Context, string) (Download, error)
	CaptureLineage(context.Context, LineageRequest) (Lineage, error)
}
type Writer interface {
	WriteFlow(context.Context, Artifact) (ArtifactResult, error)
}
type Action struct {
	reader Reader
	writer Writer
}

func New(reader Reader, writer Writer) *Action { return &Action{reader: reader, writer: writer} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, errors.New("flow pull dependencies are not configured")
	}
	if input.Workspace == "" {
		return Output{}, errors.New("flow pull requires a workspace")
	}
	flow, err := a.reader.ResolveFlow(ctx, input.Selector)
	if err != nil {
		return Output{}, err
	}
	download, err := a.reader.DownloadFlow(ctx, flow.LUID)
	if err != nil {
		return Output{}, err
	}
	lineage, lineageErr := a.reader.CaptureLineage(ctx, LineageRequest{Kind: "flow", RESTLUID: flow.LUID, Direction: "both", Depth: 1})
	warnings := []string{}
	if lineageErr != nil {
		lineage = Lineage{Complete: false, Direction: "both", Depth: 1}
		warnings = append(warnings, "Lineage capture was incomplete. Use lineage.pull or --full for bounded diagnostics.")
	}
	result, err := a.writer.WriteFlow(ctx, Artifact{Workspace: input.Workspace, Filename: download.Filename, Content: download.Content, Name: flow.Name, TableauID: flow.LUID, Environment: input.Environment, Site: input.Site, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID, ProjectName: flow.ProjectPath, ProjectID: flow.ProjectLUID, FileType: flow.FileType, Lineage: lineage, Overwrite: input.Overwrite})
	if err != nil {
		return Output{}, err
	}
	if err := normalizeArtifactPaths(&result); err != nil {
		return Output{}, err
	}
	result.LineageStatus = lineageStatus(lineage)
	result.NodeCount = len(lineage.Nodes)
	result.EdgeCount = len(lineage.Edges)
	result.CountsKnown = lineageErr == nil
	warnings = append(warnings, result.Warnings...)
	return Output{Status: "pulled", Flow: flow, Artifact: result, Warnings: warnings, RequestID: download.TableauRequestID, Help: []string{"tadx content flow publish --artifact " + result.Path}}, nil
}
func lineageStatus(value Lineage) string {
	if value.Complete {
		return "complete"
	}
	return "incomplete"
}

func normalizeArtifactPaths(result *ArtifactResult) error {
	paths := []struct {
		name  string
		value *string
	}{
		{name: "artifact path", value: &result.Path},
		{name: "canonical path", value: &result.CanonicalPath},
		{name: "lineage path", value: &result.LineagePath},
	}
	for _, candidate := range paths {
		name, value := candidate.name, candidate.value
		if *value == "" {
			continue
		}
		if filepath.IsAbs(*value) || filepath.VolumeName(*value) != "" {
			return errors.New("flow artifact writer returned an absolute " + name)
		}
		normalized := strings.ReplaceAll(*value, "\\", "/")
		for _, segment := range strings.Split(normalized, "/") {
			if segment == ".." {
				return errors.New("flow artifact writer returned an escaping " + name)
			}
		}
		*value = normalized
	}
	return nil
}
