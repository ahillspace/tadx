package app

import (
	"context"
	"errors"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestCatalogGlobalSearchIncludesReadThroughResourcesWithoutGeneration(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(t.TempDir(), func() time.Time { return now })
	if err := store.UpsertResources(context.Background(), []catalog.ResourceEntry{{Environment: "dev", Site: "site", Kind: "workbook", LUID: "wb-1", Name: "Sales", Coverage: "summary", ObservedAt: now}}); err != nil {
		t.Fatal(err)
	}
	out, err := searchaction.New(catalogGlobalSearchSource{store: store}).Execute(context.Background(), searchaction.Input{Terms: "sales", Type: "workbook", Environment: "dev", Site: "site", SiteResolved: true, Catalog: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].LUID != "wb-1" || out.Generation != nil {
		t.Fatalf("output=%+v", out)
	}
}

func TestCatalogGlobalSearchRejectsExplicitUnavailableType(t *testing.T) {
	store := catalog.NewStore(t.TempDir(), time.Now)
	_, err := searchaction.New(catalogGlobalSearchSource{store: store}).Execute(context.Background(), searchaction.Input{Type: "metric", Environment: "dev", Site: "site", SiteResolved: true, Catalog: true})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error=%v", err)
	}
}
