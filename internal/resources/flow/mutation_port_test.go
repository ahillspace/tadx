package flow_test

import (
	"context"
	"errors"
	"testing"

	flowops "github.com/ahillspace/tadx/actions/flow"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

type flowChanges struct {
	request tableauflow.UpdateRequest
	result  tableauflow.MutationResult
	err     error
}

func (c *flowChanges) Update(_ context.Context, request tableauflow.UpdateRequest) (tableauflow.MutationResult, error) {
	c.request = request
	return c.result, c.err
}
func (c *flowChanges) Move(context.Context, string, string) (tableauflow.MutationResult, error) {
	return c.result, c.err
}
func (c *flowChanges) Delete(context.Context, string) (tableauflow.MutationResult, error) {
	return c.result, c.err
}

func TestMutationPortPreservesFlowReceiptAndMatchingObservation(t *testing.T) {
	owner := "owner-2"
	changes := &flowChanges{result: tableauflow.MutationResult{Status: "succeeded", FlowLUID: "flow-1", ProjectLUID: "project-2", OwnerLUID: owner, TableauRequestID: "request-flow"}}
	port := resourceflow.NewMutationPort(nil, nil, changes)
	move, err := port.MoveFlow(t.Context(), "flow-1", "project-2")
	if err != nil || move.FlowLUID != "flow-1" || move.ProjectLUID != "project-2" || move.TableauRequestID != "request-flow" {
		t.Fatalf("move=%#v err=%v", move, err)
	}
	current := flowops.Record{LUID: "flow-1", Name: "Daily Prep", ProjectLUID: "project-1"}
	updated, err := port.UpdateFlow(t.Context(), current, flowops.UpdateRequest{LUID: "flow-1", OwnerLUID: &owner})
	if err != nil || changes.request.OwnerLUID != &owner || updated.FlowName != "Daily Prep" || updated.ProjectLUID != "project-1" || updated.OwnerLUID != owner || updated.TableauRequestID != "request-flow" {
		t.Fatalf("update=%#v request=%#v err=%v", updated, changes.request, err)
	}
	changes.result.FlowLUID = "different"
	updated, err = port.UpdateFlow(t.Context(), current, flowops.UpdateRequest{LUID: "flow-1", OwnerLUID: &owner})
	if err != nil || updated.FlowName != "" || updated.ProjectLUID != "" {
		t.Fatalf("mismatched update=%#v err=%v", updated, err)
	}
	changes.result.FlowLUID = "flow-1"
	deleted, err := port.DeleteFlow(t.Context(), "flow-1")
	if err != nil || deleted.FlowLUID != "flow-1" || deleted.TableauRequestID != "request-flow" {
		t.Fatalf("delete=%#v err=%v", deleted, err)
	}
	cause := errors.New("receipt uncertain")
	changes.err = cause
	move, err = port.MoveFlow(t.Context(), "flow-1", "project-2")
	if !errors.Is(err, cause) || move.FlowLUID != "flow-1" || move.TableauRequestID != "request-flow" {
		t.Fatalf("partial move=%#v err=%v", move, err)
	}
}
