package workbook

import (
	"context"
	"errors"
	"testing"

	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type publishDestinationClient struct{}

func (publishDestinationClient) Get(context.Context, string) (tableauworkbook.Workbook, error) {
	return tableauworkbook.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1"}, nil
}
func (publishDestinationClient) List(context.Context, int, int) (tableauworkbook.WorkbookPage, error) {
	panic("unexpected list")
}
func (publishDestinationClient) ListProjects(context.Context, int, int) (tableauworkbook.ProjectPage, error) {
	panic("unexpected project list")
}
func (publishDestinationClient) Download(context.Context, string, *bool) (tableauworkbook.Download, error) {
	panic("unexpected download")
}
func (publishDestinationClient) Prepare(context.Context, tableauworkbook.PublishRequest) (*tableauworkbook.PreparedPublish, error) {
	panic("unexpected prepare")
}

type publishDestinationProjectPaths struct{ err error }

func (p publishDestinationProjectPaths) ResolveProjectPath(context.Context, string) (string, error) {
	return "", p.err
}

func TestPublishDestinationRetainsPartialIdentityWhenProjectPathReadFails(t *testing.T) {
	pathFailure := errors.New("project path unavailable")
	adapter := NewAdapterWithProjectResolver(publishDestinationClient{}, publishDestinationProjectPaths{err: pathFailure})
	result, err := adapter.ConfirmPublicationDestination(t.Context(), "wb-1", "Finance", "project-1")
	if !errors.Is(err, pathFailure) || result.ResourceID != "wb-1" || result.Name != "Finance" || result.ProjectID != "project-1" {
		t.Fatalf("destination=%#v err=%v", result, err)
	}
}
