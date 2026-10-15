package value

import (
	"testing"
)

func TestFieldMetadataPreservesRawIdentityAndRejectsCaptionJoin(t *testing.T) {
	fields := []SchemaField{{ID: "raw-sales", Name: "[Sales]", Caption: "Revenue", Role: "measure"}, {ID: "raw-cost", Name: "[Cost]", Caption: "Revenue"}}
	metadata := []FieldDescription{{MetadataID: "graphql-1", FullyQualifiedName: "[Sales]", Name: "Revenue"}, {MetadataID: "graphql-2", FullyQualifiedName: "Revenue", Name: "Revenue"}}
	got := EnrichSchemaFields(fields, metadata)
	if got[0].ID != "raw-sales" || got[0].Metadata.MetadataID != "graphql-1" || got[1].Metadata != nil || got[1].MetadataMatch != "unmatched" {
		t.Fatalf("unexpected join: %#v", got)
	}
	got[0].Role = "dimension"
	got[0].Metadata.Name = "changed"
	if fields[0].Role != "measure" || fields[0].Metadata != nil || metadata[0].Name != "Revenue" {
		t.Fatal("field metadata enrichment changed the provider records")
	}
	metadata = append(metadata, FieldDescription{MetadataID: "graphql-3", FullyQualifiedName: "[Sales]"})
	got = EnrichSchemaFields(fields, metadata)
	if got[0].Metadata != nil || got[0].MetadataMatch != "ambiguous" {
		t.Fatalf("ambiguous join accepted: %#v", got[0])
	}
}

func TestFieldMetadataMatchesSimpleTableauIdentifierQuoting(t *testing.T) {
	for _, raw := range []string{"facility_city", "Sales Amount"} {
		got := EnrichSchemaFields([]SchemaField{{ID: raw, Name: raw, Caption: "unrelated"}}, []FieldDescription{{MetadataID: "m1", FullyQualifiedName: "[" + raw + "]"}})
		if got[0].ID != raw || got[0].Metadata == nil || got[0].MetadataMatch != "exact" {
			t.Fatalf("quote join: %#v", got)
		}
	}
	if schemaMetadataIdentifier("[Orders].[Sales]") != "[Orders].[Sales]" {
		t.Fatal("qualified path was guessed")
	}
}
