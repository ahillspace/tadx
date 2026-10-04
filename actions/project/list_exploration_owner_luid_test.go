package project

import (
	"context"
	"testing"
)

const listExplorationOwnerLUID = "1f876ad6-d65f-4b4e-9c67-3fbbe38fdd37"

type listExplorationProjectReader struct{ nameReads, inventoryReads int }

func (r *listExplorationProjectReader) ListProjects(_ context.Context, input ListPageRequest) (ListPage, error) {
	if input.OwnerName != "" {
		r.nameReads++
		return ListPage{Number: input.PageNumber, Size: input.PageSize, Total: 0}, nil
	}
	r.inventoryReads++
	return ListPage{Number: input.PageNumber, Size: input.PageSize, Total: 3, Projects: []ListProject{
		{LUID: "p1", OwnerLUID: listExplorationOwnerLUID},
		{LUID: "p2", OwnerLUID: "other"},
		{LUID: "p3", OwnerLUID: listExplorationOwnerLUID},
	}}, nil
}

func TestListExplorationOwnerLUIDContinuationSkipsNameFilter(t *testing.T) {
	reader := &listExplorationProjectReader{}
	action := newInternalTestService(Ports{ListReader: reader})
	first, err := action.ListProjects(t.Context(), ListInput{Environment: "test", OwnerName: listExplorationOwnerLUID, Limit: 1})
	if err != nil || len(first.Projects) != 1 || first.Projects[0].LUID != "p1" || first.Page.Total != 2 || first.Page.NextCursor == "" {
		t.Fatalf("first page: %#v, %v", first, err)
	}
	second, err := action.ListProjects(t.Context(), ListInput{Environment: "test", OwnerName: listExplorationOwnerLUID, Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Projects) != 1 || second.Projects[0].LUID != "p3" || second.Page.MoreAvailable {
		t.Fatalf("second page: %#v, %v", second, err)
	}
	if reader.nameReads != 1 || reader.inventoryReads != 2 {
		t.Fatalf("name reads=%d inventory reads=%d", reader.nameReads, reader.inventoryReads)
	}
}

func TestListExplorationOwnerLUIDCacheSubsetCannotClaimComplete(t *testing.T) {
	reader := &listExplorationProjectReader{}
	_, err := newInternalTestService(Ports{ListReader: reader}).ListProjects(t.Context(), ListInput{Environment: "test", OwnerName: listExplorationOwnerLUID, Cache: true})
	if err == nil || reader.inventoryReads != 0 {
		t.Fatalf("cache owner LUID fallback must fail before subset inventory: %v, reads=%d", err, reader.inventoryReads)
	}
}

func TestListExplorationOwnerLUIDAllUsesCompleteInventory(t *testing.T) {
	reader := &listExplorationProjectReader{}
	out, err := newInternalTestService(Ports{ListReader: reader}).ListProjects(t.Context(), ListInput{Environment: "test", OwnerName: listExplorationOwnerLUID, All: true})
	if err != nil || len(out.Projects) != 2 || out.Page.Total != 2 || reader.inventoryReads != 1 {
		t.Fatalf("all owner projects: %#v, %v, reads=%d", out, err, reader.inventoryReads)
	}
}
