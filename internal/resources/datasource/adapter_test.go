package datasource_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type client struct {
	metadata    tableaudatasource.Datasource
	download    tableaudatasource.Download
	getErr      error
	downloadErr error
	calls       []string
}

func (c *client) Get(_ context.Context, luid string) (tableaudatasource.Datasource, error) {
	c.calls = append(c.calls, "get:"+luid)
	return c.metadata, c.getErr
}

func (c *client) Download(_ context.Context, luid string, includeExtract *bool) (tableaudatasource.Download, error) {
	c.calls = append(c.calls, "download:"+luid)
	if includeExtract != nil {
		panic("dependency acquisition must preserve the server's native extract behavior")
	}
	return c.download, c.downloadErr
}

func TestAdapterDownloadsOneAuthoritativeDatasourceArtifact(t *testing.T) {
	c := &client{
		metadata: tableaudatasource.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Shared"},
		download: tableaudatasource.Download{Filename: "Sales.tdsx", Content: []byte("native"), TableauRequestID: "request-1"},
	}

	result, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.calls, []string{"get:ds-1", "download:ds-1"}) {
		t.Fatalf("calls = %v", c.calls)
	}
	if result.LUID != "ds-1" || result.Name != "Sales" || result.ProjectLUID != "project-1" || result.ProjectPath != "Shared" || result.Filename != "Sales.tdsx" || string(result.Content) != "native" || result.TableauRequestID != "request-1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestAdapterStopsBeforeDownloadWhenAuthoritativeReadFails(t *testing.T) {
	c := &client{getErr: errors.New("not found")}
	_, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1")
	if err == nil || !reflect.DeepEqual(c.calls, []string{"get:ds-1"}) {
		t.Fatalf("error = %v, calls = %v", err, c.calls)
	}
}

func TestAdapterRejectsIncompleteDatasourceIdentity(t *testing.T) {
	for _, metadata := range []tableaudatasource.Datasource{
		{Name: "Sales"},
		{LUID: "ds-other", Name: "Sales"},
		{LUID: "ds-1"},
		{LUID: "ds-1", Name: "Sales"},
	} {
		c := &client{metadata: metadata}
		_, err := resourcedatasource.NewAdapter(c).DownloadDatasource(context.Background(), "ds-1")
		if err == nil || !reflect.DeepEqual(c.calls, []string{"get:ds-1"}) {
			t.Fatalf("metadata = %#v, error = %v, calls = %v", metadata, err, c.calls)
		}
	}
}
