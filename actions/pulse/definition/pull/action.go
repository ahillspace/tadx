package pull

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pathspec"
)

// Reader is the action-owned exact definition seam.
type Reader interface {
	GetDefinition(context.Context, string) (Definition, error)
}

// Writer is the action-owned managed artifact seam.
type Writer interface {
	WriteDefinition(context.Context, Artifact) (ArtifactResult, error)
}

// Action pulls one Pulse definition into a managed artifact.
type Action struct {
	reader Reader
	writer Writer
}

// New creates a Pulse definition pull action.
func New(reader Reader, writer Writer) *Action { return &Action{reader: reader, writer: writer} }

// Execute retrieves and atomically materializes one definition.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, pullError("pulse.definition.pull.unconfigured", errs.KindRuntime, input, "Pulse definition pull is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if strings.TrimSpace(input.Workspace) == "" || input.LUID == "" {
		return Output{}, pullError("pulse.definition.pull.usage", errs.KindUsage, input, "Pulse definition pull requires a workspace and exact definition LUID.", nil)
	}
	definition, err := a.reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact definition LUID and selected Tableau site, then retry.")
		return Output{}, &errs.Error{ID: "pulse.definition.pull.read", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse definition retrieval failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if definition.LUID != input.LUID || definition.Name == "" || definition.DatasourceLUID == "" || len(definition.Configuration) == 0 {
		return Output{}, pullError("pulse.definition.pull.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched Pulse definition.", errors.New("definition requires matching LUID, name, datasource LUID, and configuration"))
	}
	artifact, err := a.writer.WriteDefinition(ctx, Artifact{
		Workspace: input.Workspace, DefinitionLUID: definition.LUID, Name: definition.Name, DatasourceLUID: definition.DatasourceLUID,
		Environment: input.Environment, Site: input.Site, ServerOrigin: input.ServerOrigin, SiteLUID: input.SiteLUID,
		Configuration: append([]byte(nil), definition.Configuration...), Overwrite: input.Overwrite,
	})
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Review the workspace and managed artifact target, then pull again.")
		return Output{}, &errs.Error{ID: "pulse.definition.pull.write", Kind: errs.KindOperation, Operation: "pulse.definition.pull", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse definition artifact write failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective}
	}
	if err := normalizeArtifactPaths(&artifact); err != nil {
		return Output{}, pullError("pulse.definition.pull.normalize", errs.KindOperation, input, "Pulse definition artifact path normalization failed.", err)
	}
	return Output{Status: "pulled", Definition: definition, Artifact: artifact, RequestID: definition.RequestID, Help: []string{"Inspect " + artifact.CanonicalPath + "."}}, nil
}

func normalizeArtifactPaths(result *ArtifactResult) error {
	for _, candidate := range []*string{&result.Path, &result.CanonicalPath} {
		if *candidate == "" {
			continue
		}
		if pathspec.IsAbs(*candidate) {
			return errors.New("artifact writer returned an absolute path")
		}
		normalized := strings.ReplaceAll(*candidate, "\\", "/")
		for _, segment := range strings.Split(normalized, "/") {
			if segment == ".." {
				return errors.New("artifact writer returned an escaping path")
			}
		}
		*candidate = normalized
	}
	return nil
}

func pullError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.definition.pull", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact Pulse definition and a managed logical workspace."}
}
