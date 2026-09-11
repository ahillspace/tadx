package datasource

import (
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

func TestFieldMetadataPreservesRawIdentityAndRejectsCaptionJoin(t *testing.T) {
	fields := []value.SchemaField{{ID: "raw-sales", Name: "[Sales]", Caption: "Revenue", Role: "measure"}, {ID: "raw-cost", Name: "[Cost]", Caption: "Revenue"}}
	metadata := []value.FieldDescription{{MetadataID: "graphql-1", FullyQualifiedName: "[Sales]", Name: "Revenue"}, {MetadataID: "graphql-2", FullyQualifiedName: "Revenue", Name: "Revenue"}}
	got := EnrichFields(fields, metadata)
	if got[0].ID != "raw-sales" || got[0].Metadata.MetadataID != "graphql-1" || got[1].Metadata != nil || got[1].MetadataMatch != "unmatched" {
		t.Fatalf("unexpected join: %#v", got)
	}
	metadata = append(metadata, value.FieldDescription{MetadataID: "graphql-3", FullyQualifiedName: "[Sales]"})
	got = EnrichFields(fields, metadata)
	if got[0].Metadata != nil || got[0].MetadataMatch != "ambiguous" {
		t.Fatalf("ambiguous join accepted: %#v", got[0])
	}
}

func TestFieldMetadataMatchesSimpleTableauIdentifierQuoting(t *testing.T) {
	for _, raw := range []string{"facility_city", "Sales Amount"} {
		got := EnrichFields([]value.SchemaField{{ID: raw, Name: raw, Caption: "unrelated"}}, []value.FieldDescription{{MetadataID: "m1", FullyQualifiedName: "[" + raw + "]"}})
		if got[0].ID != raw || got[0].Metadata == nil || got[0].MetadataMatch != "exact" {
			t.Fatalf("quote join: %#v", got)
		}
	}
	if metadataIdentifier("[Orders].[Sales]") != "[Orders].[Sales]" {
		t.Fatal("qualified path was guessed")
	}
}
