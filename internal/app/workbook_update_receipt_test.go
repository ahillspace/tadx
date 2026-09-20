package app

import (
	"testing"

	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
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
		result, err := (workbookUpdateAdapter{workbooks: resourceworkbook.NewAdapter(client)}).UpdateWorkbook(t.Context(), workbookupdate.Request{LUID: "wb-1", Description: description})
		if err != nil {
			t.Fatal(err)
		}
		if result.Description == nil || *result.Description != *description || result.EvidenceSource != "tableau_update_response" || result.TableauRequestID != "request-update" {
			t.Fatalf("adapter lost response evidence: %#v", result)
		}
	}
}
