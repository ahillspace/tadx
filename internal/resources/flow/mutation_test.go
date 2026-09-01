package flow_test

import (
	"context"
	"testing"

	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

type preparedPublish struct{}

func (preparedPublish) Commit(context.Context) (tableauflow.PublishResult, error) {
	return tableauflow.PublishResult{Status: "succeeded", FlowLUID: "f-new"}, nil
}

type mutationClient struct{ prepared, moved, deleted int }

func (c *mutationClient) Prepare(context.Context, tableauflow.PublishRequest) (tableauflow.PreparedPublish, error) {
	c.prepared++
	return preparedPublish{}, nil
}
func (c *mutationClient) Move(context.Context, string, string) (tableauflow.MutationResult, error) {
	c.moved++
	return tableauflow.MutationResult{Status: "succeeded", FlowLUID: "f-1", ProjectLUID: "p-2"}, nil
}
func (c *mutationClient) Delete(context.Context, string) (tableauflow.MutationResult, error) {
	c.deleted++
	return tableauflow.MutationResult{Status: "succeeded", FlowLUID: "f-1"}, nil
}

func TestMutationAdapterPassesOnlyExplicitFlowChanges(t *testing.T) {
	client := &mutationClient{}
	adapter := resourceflow.NewMutationAdapter(client)
	prepared, err := adapter.PrepareFlow(context.Background(), tableauflow.PublishRequest{Name: "Daily", ProjectLUID: "p-1", Filename: "Daily.tflx"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.MoveFlow(context.Background(), "f-1", "p-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.DeleteFlow(context.Background(), "f-1"); err != nil {
		t.Fatal(err)
	}
	if client.prepared != 1 || client.moved != 1 || client.deleted != 1 {
		t.Fatalf("client=%#v", client)
	}
}
