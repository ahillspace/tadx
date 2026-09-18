package app

import (
	"context"
	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	"github.com/ahillspace/tadx/internal/artifact"
	"path/filepath"
)

type nativeWorkbookArtifactReader struct{}

func (nativeWorkbookArtifactReader) ReadWorkbook(ctx context.Context, path string) (workbookpublish.Artifact, error) {
	item, err := artifact.ReadNative(ctx, path, "workbook")
	return workbookpublish.Artifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint, Portability: artifact.PortabilityUnknown}, err
}

type nativeDatasourceArtifactReader struct{}

func (nativeDatasourceArtifactReader) ReadDatasource(ctx context.Context, path string) (datasourcepublish.Artifact, error) {
	item, err := artifact.ReadNative(ctx, path, "datasource")
	return datasourcepublish.Artifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint, CompositionStatus: item.CompositionStatus, ParentDataSourceURLs: item.ParentDataSourceURLs}, err
}

type nativeFlowArtifactReader struct{}

func (nativeFlowArtifactReader) ReadFlow(ctx context.Context, path string) (flowpublish.Artifact, error) {
	item, err := artifact.ReadNative(ctx, path, "flow")
	return flowpublish.Artifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint}, err
}
