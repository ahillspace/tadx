package inspect

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type explorationPartialDatabaseReader struct{}

func (explorationPartialDatabaseReader) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	return value.MetadataDatabase{}, errors.New("unavailable")
}

func (explorationPartialDatabaseReader) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	return value.MetadataPage[value.MetadataDatabase]{Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{MetadataID: "metadata-db", Name: "Confirmed"}}}}, errors.New("requested coverage unavailable")
}

func TestExplorationCatalogConfirmedItemRemainsPartial(t *testing.T) {
	out, err := New(explorationPartialDatabaseReader{}).Execute(t.Context(), Input{MetadataID: "metadata-db"})
	if err == nil || out.Status != "partial" || out.Item == nil || out.Item.Name != "Confirmed" {
		t.Fatalf("partial inspection evidence: %#v, %v", out, err)
	}
	compact := out.CompactOutput()
	if compact == nil || out.FullOutput() == nil {
		t.Fatal("partial inspection projection disappeared")
	}
}
