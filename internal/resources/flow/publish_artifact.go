package flow

import (
	"context"
	"path/filepath"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/artifact"
)

// NativePublishReader reads a caller-selected native flow file.
type NativePublishReader struct{}

func (NativePublishReader) ReadFlow(ctx context.Context, path string) (flow.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "flow")
	return flow.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint}, err
}

// ManagedPublishReader projects a managed flow without changing its payload.
type ManagedPublishReader struct {
	Manager     *artifact.FlowManager
	DisplayPath string
}

func (r ManagedPublishReader) ReadFlow(ctx context.Context, path string) (flow.PublishArtifact, error) {
	item, err := r.Manager.Read(ctx, path)
	return flow.PublishArtifact{TableauID: item.Metadata.TableauID, Path: r.DisplayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, Fingerprint: item.Fingerprint, SourceEnvironment: item.Metadata.SourceEnvironment, SourceSite: item.Metadata.SourceSite, SourceProjectName: item.Metadata.SourceProjectName, SourceProjectID: item.Metadata.SourceProjectID, Size: item.Size}, err
}
