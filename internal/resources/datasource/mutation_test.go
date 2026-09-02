package datasource_test

import (
	"context"
	"testing"

	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type preparedDatasourcePublish struct{}

func (preparedDatasourcePublish) Commit(context.Context) (tableaudatasource.PublishResult, error) {
	return tableaudatasource.PublishResult{Status: "succeeded", DatasourceLUID: "ds-new"}, nil
}

type datasourceMutationClient struct {
	prepared, deleted int
	request           tableaudatasource.PublishRequest
}

func (c *datasourceMutationClient) Prepare(_ context.Context, input tableaudatasource.PublishRequest) (tableaudatasource.PreparedPublish, error) {
	c.prepared++
	c.request = input
	return preparedDatasourcePublish{}, nil
}
func (c *datasourceMutationClient) Delete(context.Context, string) (tableaudatasource.MutationResult, error) {
	c.deleted++
	return tableaudatasource.MutationResult{Status: "succeeded", DatasourceLUID: "ds-1"}, nil
}

func TestMutationAdapterPassesOnlyExplicitDatasourceChanges(t *testing.T) {
	client := &datasourceMutationClient{}
	adapter := resourcedatasource.NewMutationAdapter(client)
	prepared, err := adapter.PrepareDatasource(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", AsJob: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.DeleteDatasource(context.Background(), "ds-1"); err != nil {
		t.Fatal(err)
	}
	if client.prepared != 1 || client.deleted != 1 || !client.request.AsJob {
		t.Fatalf("client = %#v", client)
	}
}
