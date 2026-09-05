package datasource_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

type importedProjectInventory struct{}

func (importedProjectInventory) List(_ context.Context, in tableauproject.ListRequest) (tableauproject.Page, error) {
	return tableauproject.Page{Number: in.PageNumber, Size: in.PageSize, Total: 2, Items: []tableauproject.Project{
		{LUID: "system-project", Name: "(imported)"},
		{LUID: "real-project", Name: "Imported"},
	}}, nil
}

func TestImportedAliasCannotRedirectDatasourceFromARealProject(t *testing.T) {
	for _, realDatasourceExists := range []bool{false, true} {
		items := []tableaudatasource.Datasource{{LUID: "system-ds", Name: "Sales", ProjectLUID: "system-project", ProjectName: "(imported)"}}
		if realDatasourceExists {
			items = append(items, tableaudatasource.Datasource{LUID: "real-ds", Name: "Sales", ProjectLUID: "real-project", ProjectName: "Imported"})
		}
		c := &client{pages: map[int]tableaudatasource.Page{1: {Number: 1, Size: 1000, Total: len(items), Items: items}}}
		adapter := resourcedatasource.NewAdapterWithProjectResolver(c, resourceproject.NewAdapter(importedProjectInventory{}))
		item, err := adapter.ResolveDatasource(context.Background(), identity.Selector{Name: "Sales", ProjectPath: "Imported"})
		if realDatasourceExists {
			if err != nil || item.LUID != "real-ds" || item.ProjectLUID != "real-project" || item.ProjectPath != "Imported" {
				t.Fatalf("real project identity changed: item=%+v err=%v", item, err)
			}
		} else {
			var resolution *identity.ResolutionError
			if !errors.As(err, &resolution) || resolution.Kind != identity.ResolutionNotFound {
				t.Fatalf("missing datasource in real project fell back to imported content: item=%+v err=%v", item, err)
			}
		}
	}
}
