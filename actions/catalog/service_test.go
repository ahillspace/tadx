package catalog

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type recordingProvider struct{ opens int }

func (p *recordingProvider) Open(context.Context, string, string, bool) (Target, error) {
	p.opens++
	return Target{}, nil
}

type selectedProvider struct {
	assets                 Assets
	operation, environment string
	explicit               bool
}

func (p *selectedProvider) Open(_ context.Context, operation, environment string, explicit bool) (Target, error) {
	p.operation, p.environment, p.explicit = operation, environment, explicit
	return Target{Environment: "canonical", Site: "exact-site", Assets: p.assets}, nil
}

type selectedAssets struct {
	Assets
	reads int
}

func (a *selectedAssets) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	a.reads++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "db"}}, nil
}

func TestServiceBindsCanonicalTargetForUpdatePreview(t *testing.T) {
	assets := &selectedAssets{}
	provider := &selectedProvider{assets: assets}
	service := New(provider)
	description := "Revised"
	out, err := service.UpdateCatalogDatabase(t.Context(), DatabaseInput{Environment: "selected", ID: "db", Description: &description}, true)
	if err != nil {
		t.Fatal(err)
	}
	if provider.operation != "catalog.database.update" || provider.environment != "selected" || !provider.explicit {
		t.Fatalf("provider selection changed: %+v", provider)
	}
	if out.Plan.Environment != "canonical" || out.Plan.Site != "exact-site" || assets.reads != 1 {
		t.Fatalf("preview target or read sequence changed: plan=%+v reads=%d", out.Plan, assets.reads)
	}
}

func TestServiceValidatesBeforeOpeningTarget(t *testing.T) {
	provider := &recordingProvider{}
	service := New(provider)
	invalid := []struct {
		name string
		run  func() error
	}{
		{"database list", func() error {
			_, err := service.ListCatalogDatabases(t.Context(), DatabaseListInput{Limit: -1})
			return err
		}},
		{"database inspect", func() error {
			_, err := service.InspectCatalogDatabase(t.Context(), DatabaseInspectInput{})
			return err
		}},
		{"database update", func() error { _, err := service.UpdateCatalogDatabase(t.Context(), DatabaseInput{}, false); return err }},
		{"table list", func() error { _, err := service.ListCatalogTables(t.Context(), TableListInput{Limit: -1}); return err }},
		{"table inspect", func() error { _, err := service.InspectCatalogTable(t.Context(), TableInspectInput{}); return err }},
		{"table update", func() error { _, err := service.UpdateCatalogTable(t.Context(), TableInput{}, false); return err }},
		{"column list", func() error { _, err := service.ListCatalogColumns(t.Context(), ColumnListInput{}); return err }},
		{"column inspect", func() error { _, err := service.InspectCatalogColumn(t.Context(), ColumnInspectInput{}); return err }},
		{"column update", func() error { _, err := service.UpdateCatalogColumn(t.Context(), ColumnInput{}, false); return err }},
		{"search", func() error { _, err := service.SearchCatalog(t.Context(), SearchInput{}); return err }},
		{"audit", func() error { _, err := service.AuditCatalog(t.Context(), AuditInput{}); return err }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("expected validation error")
			}
			if provider.opens != 0 {
				t.Fatalf("opened target before validation: %d", provider.opens)
			}
		})
	}
}
