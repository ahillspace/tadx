package datasource

import (
	"context"
	"errors"
	"strings"

	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

// MutationClient is the temporary publication Prepare seam.
// The publication and recovery slice removes it after receipt fixtures cover that boundary.
type MutationClient interface {
	Prepare(context.Context, tableaudatasource.PublishRequest) (tableaudatasource.PreparedPublish, error)
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
