package app

import (
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	"testing"
)

func TestWorkbookUpdateAdapterPreservesResponseDescriptionEvidence(t *testing.T) {
	for _, description := range []*string{new("Confirmed"), new("")} {
		client := &contentMutationWorkbookClient{result: tableauworkbook.MutationResult{
			Status:           "succeeded",
			WorkbookLUID:     "wb-1",
			WorkbookName:     "Finance",
			ProjectLUID:      "project-1",
			OwnerLUID:        "owner-1",
			Description:      description,
			EvidenceSource:   "tableau_update_response",
			TableauRequestID: "request-update",
		}}
		result, err := (workbookMutationAdapter{workbooks: resourceworkbook.NewAdapter(client)}).UpdateWorkbook(t.Context(), workbookops.UpdateRequest{LUID: "wb-1", Description: description})
		if err != nil {
			t.Fatal(err)
		}
		if result.Description == nil || *result.Description != *description || result.EvidenceSource != "tableau_update_response" || result.TableauRequestID != "request-update" {
			t.Fatalf("adapter lost response evidence: %#v", result)
		}
	}
}
