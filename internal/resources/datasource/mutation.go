package datasource

import (
	"context"
	"errors"
	"strings"

	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

type MutationClient interface {
	Prepare(context.Context, tableaudatasource.PublishRequest) (tableaudatasource.PreparedPublish, error)
	Delete(context.Context, string) (tableaudatasource.MutationResult, error)
}

type updateClient interface {
	Update(context.Context, tableaudatasource.UpdateRequest) (tableaudatasource.MutationResult, error)
}

func (a *MutationAdapter) UpdateDatasource(ctx context.Context, input tableaudatasource.UpdateRequest) (tableaudatasource.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.LUID) == "" {
		return tableaudatasource.MutationResult{}, errors.New("datasource update requires a configured client and exact datasource LUID")
	}
	client, ok := a.client.(updateClient)
	if !ok {
		return tableaudatasource.MutationResult{}, errors.New("datasource update client is not configured")
	}
	return client.Update(ctx, input)
}

type MutationAdapter struct{ client MutationClient }

func NewMutationAdapter(client MutationClient) *MutationAdapter {
	return &MutationAdapter{client: client}
}

func (a *MutationAdapter) PrepareDatasource(ctx context.Context, input tableaudatasource.PublishRequest) (tableaudatasource.PreparedPublish, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ProjectLUID) == "" {
		return nil, errors.New("datasource publish requires a configured client, name, and project LUID")
	}
	return a.client.Prepare(ctx, input)
}

func (a *MutationAdapter) DeleteDatasource(ctx context.Context, luid string) (tableaudatasource.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(luid) == "" {
		return tableaudatasource.MutationResult{}, errors.New("datasource delete requires a configured client and exact datasource LUID")
	}
	return a.client.Delete(ctx, luid)
}
