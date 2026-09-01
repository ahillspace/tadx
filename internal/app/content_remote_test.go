package app

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type lineageDatasourceClient struct {
	page tableaudatasource.Page
}

func (c lineageDatasourceClient) Get(context.Context, string) (tableaudatasource.Datasource, error) {
	return tableaudatasource.Datasource{}, errors.New("unexpected get")
}

func (c lineageDatasourceClient) List(context.Context, tableaudatasource.ListRequest) (tableaudatasource.Page, error) {
	return c.page, nil
}

func (c lineageDatasourceClient) Download(context.Context, string, *bool) (tableaudatasource.Download, error) {
	return tableaudatasource.Download{}, errors.New("unexpected download")
}

type lineageProjectPaths map[string]string

func (p lineageProjectPaths) ResolveProjectPath(_ context.Context, luid string) (string, error) {
	path, exists := p[luid]
	if !exists {
		return "", errors.New("project not found")
	}
	return path, nil
}

func TestLineageResolverResolvesPublishedDatasourceByExactNameAndProjectPath(t *testing.T) {
	datasources := resourcedatasource.NewAdapterWithProjectResolver(lineageDatasourceClient{page: tableaudatasource.Page{
		Number: 1,
		Size:   1,
		Total:  1,
		Items: []tableaudatasource.Datasource{{
			LUID: "datasource-1", Name: "Sales", ProjectLUID: "project-1", ProjectName: "Ops",
		}},
	}}, lineageProjectPaths{"project-1": "Department/Ops"})

	resource, err := (lineageResolver{datasources: datasources}).ResolveLineageResource(
		context.Background(),
		"published_datasource",
		identity.Selector{Name: "Sales", ProjectPath: "Department/Ops"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if resource.Kind != "published_datasource" || resource.LUID != "datasource-1" || resource.Name != "Sales" || resource.ProjectPath != "Department/Ops" {
		t.Fatalf("resource = %#v", resource)
	}
}
