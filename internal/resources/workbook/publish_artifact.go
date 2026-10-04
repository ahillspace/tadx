package workbook

import (
	"context"
	"path/filepath"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
)

// NativePublishReader reads a caller-selected native workbook file.
type NativePublishReader struct{}

func (NativePublishReader) ReadWorkbook(ctx context.Context, path string) (workbook.PublishArtifact, error) {
	item, err := artifact.ReadNative(ctx, path, "workbook")
	return workbook.PublishArtifact{Path: filepath.ToSlash(item.Path), PayloadPath: item.Path, Filename: item.Filename, Name: item.Name, Size: item.Size, Fingerprint: item.Fingerprint, Portability: artifact.PortabilityUnknown}, err
}

// ManagedPublishReader projects a managed workbook without changing its payload.
type ManagedPublishReader struct {
	Manager     *artifact.WorkbookManager
	DisplayPath string
}

func (r ManagedPublishReader) ReadWorkbook(ctx context.Context, path string) (workbook.PublishArtifact, error) {
	item, err := r.Manager.Read(ctx, path)
	if err == nil && r.DisplayPath != "" {
		item.Path = r.DisplayPath
	}
	return workbook.PublishArtifact{Path: item.Path, PayloadPath: item.PayloadPath, Filename: item.Filename, Size: item.Size, Name: item.Name, TableauID: item.TableauID, Fingerprint: item.Fingerprint, SourceEnvironment: item.SourceEnvironment, SourceSite: item.SourceSite, SourceProjectName: item.SourceProjectName, SourceProjectID: item.SourceProjectID, Portability: item.Portability, PublishedDatasourceCount: item.PublishedDatasourceCount}, err
}
