package datasource_test

import (
	"context"
	"errors"
	"testing"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type datasourceChanges struct {
	request tableaudatasource.UpdateRequest
	result  tableaudatasource.MutationResult
	err     error
}

func (c *datasourceChanges) Update(_ context.Context, request tableaudatasource.UpdateRequest) (tableaudatasource.MutationResult, error) {
	c.request = request
	return c.result, c.err
}
func (c *datasourceChanges) Delete(context.Context, string) (tableaudatasource.MutationResult, error) {
	return c.result, c.err
}

func TestMutationPortPreservesExactDatasourceFieldsAndReceipt(t *testing.T) {
	project, name, owner := "project-2", "Renamed", "owner-2"
	changes := &datasourceChanges{result: tableaudatasource.MutationResult{Status: "succeeded", DatasourceLUID: "ds-1", DatasourceName: name, ProjectLUID: project, OwnerLUID: owner, TableauRequestID: "request-ds"}}
	port := resourcedatasource.NewMutationPort(nil, nil, changes)
	move, err := port.MoveDatasource(t.Context(), "ds-1", project)
	if err != nil || changes.request.LUID != "ds-1" || changes.request.ProjectLUID == nil || *changes.request.ProjectLUID != project || changes.request.Name != nil || changes.request.OwnerLUID != nil || move.DatasourceLUID != "ds-1" || move.ProjectLUID != project || move.TableauRequestID != "request-ds" {
		t.Fatalf("move=%#v request=%#v err=%v", move, changes.request, err)
	}
	updated, err := port.UpdateDatasource(t.Context(), datasourceops.UpdateRequest{LUID: "ds-1", Name: &name, OwnerLUID: &owner})
	if err != nil || changes.request.ProjectLUID != nil || changes.request.Name != &name || changes.request.OwnerLUID != &owner || updated.DatasourceName != name || updated.OwnerLUID != owner || updated.TableauRequestID != "request-ds" {
		t.Fatalf("update=%#v request=%#v err=%v", updated, changes.request, err)
	}
	deleted, err := port.DeleteDatasource(t.Context(), "ds-1")
	if err != nil || deleted.DatasourceLUID != "ds-1" || deleted.TableauRequestID != "request-ds" {
		t.Fatalf("delete=%#v err=%v", deleted, err)
	}
	cause := errors.New("receipt uncertain")
	changes.err = cause
	move, err = port.MoveDatasource(t.Context(), "ds-1", project)
	if !errors.Is(err, cause) || move.DatasourceLUID != "ds-1" || move.TableauRequestID != "request-ds" {
		t.Fatalf("partial move=%#v err=%v", move, err)
	}
}
