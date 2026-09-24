package list

import (
	"context"
	"testing"
)

const explorationOwnerLUID = "1f876ad6-d65f-4b4e-9c67-3fbbe38fdd37"

type explorationProjectReader struct{ nameReads, inventoryReads int }

func (r *explorationProjectReader) ListProjects(_ context.Context, input PageRequest) (Page, error) {
	if input.OwnerName != "" {
		r.nameReads++
		return Page{Number: input.PageNumber, Size: input.PageSize, Total: 0}, nil
	}
	r.inventoryReads++
	return Page{Number: input.PageNumber, Size: input.PageSize, Total: 3, Projects: []Project{
		{LUID: "p1", OwnerLUID: explorationOwnerLUID},
		{LUID: "p2", OwnerLUID: "other"},
		{LUID: "p3", OwnerLUID: explorationOwnerLUID},
	}}, nil
}

func TestExplorationOwnerLUIDContinuationSkipsNameFilter(t *testing.T) {
	reader := &explorationProjectReader{}
	action := New(reader)
	first, err := action.Execute(t.Context(), Input{Environment: "test", OwnerName: explorationOwnerLUID, Limit: 1})
	if err != nil || len(first.Projects) != 1 || first.Projects[0].LUID != "p1" || first.Page.Total != 2 || first.Page.NextCursor == "" {
		t.Fatalf("first page: %#v, %v", first, err)
	}
	second, err := action.Execute(t.Context(), Input{Environment: "test", OwnerName: explorationOwnerLUID, Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Projects) != 1 || second.Projects[0].LUID != "p3" || second.Page.MoreAvailable {
		t.Fatalf("second page: %#v, %v", second, err)
	}
	if reader.nameReads != 1 || reader.inventoryReads != 2 {
		t.Fatalf("name reads=%d inventory reads=%d", reader.nameReads, reader.inventoryReads)
	}
}

func TestExplorationOwnerLUIDCacheSubsetCannotClaimComplete(t *testing.T) {
	reader := &explorationProjectReader{}
	_, err := New(reader).Execute(t.Context(), Input{Environment: "test", OwnerName: explorationOwnerLUID, Cache: true})
	if err == nil || reader.inventoryReads != 0 {
		t.Fatalf("cache owner LUID fallback must fail before subset inventory: %v, reads=%d", err, reader.inventoryReads)
	}
}

func TestExplorationOwnerLUIDAllUsesCompleteInventory(t *testing.T) {
	reader := &explorationProjectReader{}
	out, err := New(reader).Execute(t.Context(), Input{Environment: "test", OwnerName: explorationOwnerLUID, All: true})
	if err != nil || len(out.Projects) != 2 || out.Page.Total != 2 || reader.inventoryReads != 1 {
		t.Fatalf("all owner projects: %#v, %v, reads=%d", out, err, reader.inventoryReads)
	}
}
