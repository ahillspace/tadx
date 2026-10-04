package datasource

import (
	"context"
	"path/filepath"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/artifact"
)

// NativePublishReader reads a caller-selected native datasource file.
type NativePublishReader struct{}

func (NativePublishReader) ReadDatasource(ctx context.Context, path string) (datasource.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "datasource")
	return datasource.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint, CompositionStatus: item.CompositionStatus, ParentDataSourceURLs: item.ParentDataSourceURLs}, err
}

// ManagedPublishReader projects a managed datasource without changing its payload.
type ManagedPublishReader struct {
	Manager     *artifact.DatasourceManager
	DisplayPath string
}

func (r ManagedPublishReader) ReadDatasource(ctx context.Context, path string) (datasource.PublishArtifact, error) {
	item, err := r.Manager.Read(ctx, path)
	return datasource.PublishArtifact{Path: r.DisplayPath, PayloadPath: item.PayloadPath, Filename: item.Filename, Name: item.Name, TableauID: item.TableauID, Fingerprint: item.Fingerprint, SourceEnvironment: item.SourceEnvironment, SourceSite: item.SourceSite, SourceProjectName: item.SourceProjectName, SourceProjectID: item.SourceProjectID, Size: item.Size, CompositionStatus: item.CompositionStatus, ParentDataSourceURLs: append([]string(nil), item.ParentDataSourceURLs...)}, err
}
