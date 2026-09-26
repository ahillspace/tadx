package app

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"path/filepath"
)

type nativeWorkbookArtifactReader struct{}

func (nativeWorkbookArtifactReader) ReadWorkbook(ctx context.Context, path string) (workbookops.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "workbook")
	return workbookops.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint, Portability: artifact.PortabilityUnknown}, err
}

type nativeDatasourceArtifactReader struct{}

func (nativeDatasourceArtifactReader) ReadDatasource(ctx context.Context, path string) (datasourceops.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "datasource")
	return datasourceops.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint, CompositionStatus: item.CompositionStatus, ParentDataSourceURLs: item.ParentDataSourceURLs}, err
}

type nativeFlowArtifactReader struct{}

func (nativeFlowArtifactReader) ReadFlow(ctx context.Context, path string) (flowops.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "flow")
	return flowops.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint}, err
}
