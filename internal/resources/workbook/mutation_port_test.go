package workbook_test

import (
	"context"
	"errors"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

type workbookChanges struct {
	request tableauworkbook.UpdateRequest
	result  tableauworkbook.MutationResult
	err     error
}

func (c *workbookChanges) Update(_ context.Context, request tableauworkbook.UpdateRequest) (tableauworkbook.MutationResult, error) {
	c.request = request
	return c.result, c.err
}
func (c *workbookChanges) Delete(context.Context, string) (tableauworkbook.MutationResult, error) {
	return c.result, c.err
}

func TestMutationPortPreservesExactWorkbookFieldsAndReceipt(t *testing.T) {
	project, name, owner, description := "project-2", "Renamed", "owner-2", ""
	changes := &workbookChanges{result: tableauworkbook.MutationResult{Status: "succeeded", WorkbookLUID: "wb-1", WorkbookName: name, ProjectLUID: project, OwnerLUID: owner, Description: &description, EvidenceSource: "tableau_update_response", TableauRequestID: "request-wb"}}
	port := resourceworkbook.NewMutationPort(nil, nil, changes)
	move, err := port.MoveWorkbook(t.Context(), "wb-1", project)
	if err != nil || changes.request.LUID != "wb-1" || changes.request.ProjectLUID == nil || *changes.request.ProjectLUID != project || changes.request.Name != nil || changes.request.OwnerLUID != nil || changes.request.Description != nil || move.WorkbookLUID != "wb-1" || move.ProjectLUID != project || move.TableauRequestID != "request-wb" {
		t.Fatalf("move=%#v request=%#v err=%v", move, changes.request, err)
	}
	updated, err := port.UpdateWorkbook(t.Context(), workbookops.UpdateRequest{LUID: "wb-1", Name: &name, OwnerLUID: &owner, Description: &description})
	if err != nil || changes.request.ProjectLUID != nil || changes.request.Name != &name || changes.request.OwnerLUID != &owner || changes.request.Description != &description || updated.WorkbookName != name || updated.OwnerLUID != owner || updated.Description == nil || *updated.Description != "" || updated.EvidenceSource != "tableau_update_response" || updated.TableauRequestID != "request-wb" {
		t.Fatalf("update=%#v request=%#v err=%v", updated, changes.request, err)
	}
	deleted, err := port.DeleteWorkbook(t.Context(), "wb-1")
	if err != nil || deleted.WorkbookLUID != "wb-1" || deleted.TableauRequestID != "request-wb" {
		t.Fatalf("delete=%#v err=%v", deleted, err)
	}
	cause := errors.New("receipt uncertain")
	changes.err = cause
	move, err = port.MoveWorkbook(t.Context(), "wb-1", project)
	if !errors.Is(err, cause) || move.WorkbookLUID != "wb-1" || move.TableauRequestID != "request-wb" {
		t.Fatalf("partial move=%#v err=%v", move, err)
	}
}
