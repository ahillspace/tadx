// Package datasource isolates published datasource REST behavior.
package datasource

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

// Client is the narrow Tableau datasource client used by dependency acquisition.
type Client interface {
	Get(context.Context, string) (tableaudatasource.Datasource, error)
	Download(context.Context, string, *bool) (tableaudatasource.Download, error)
}

// Download is one authoritative datasource and its unchanged native package.
type Download struct {
	LUID             string
	Name             string
	ProjectLUID      string
	ProjectPath      string
	Filename         string
	Content          []byte
	TableauRequestID string
}

// Adapter owns published datasource identity and download sequencing.
type Adapter struct{ client Client }

// NewAdapter creates a datasource resource adapter.
func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

// DownloadDatasource reads authoritative identity before downloading native content.
func (a *Adapter) DownloadDatasource(ctx context.Context, luid string) (Download, error) {
	if a == nil || a.client == nil {
		return Download{}, errors.New("datasource adapter is not configured")
	}
	luid = strings.TrimSpace(luid)
	if luid == "" {
		return Download{}, errors.New("datasource LUID is required")
	}
	item, err := a.client.Get(ctx, luid)
	if err != nil {
		return Download{}, err
	}
	if strings.TrimSpace(item.LUID) != luid {
		return Download{}, fmt.Errorf("datasource read returned LUID %q, expected %q", item.LUID, luid)
	}
	if strings.TrimSpace(item.Name) == "" {
		return Download{}, fmt.Errorf("datasource %q omitted its authoritative name", luid)
	}
	if strings.TrimSpace(item.ProjectLUID) == "" {
		return Download{}, fmt.Errorf("datasource %q omitted its authoritative project LUID", luid)
	}
	content, err := a.client.Download(ctx, luid, nil)
	if err != nil {
		return Download{}, err
	}
	return Download{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectName,
		Filename: content.Filename, Content: content.Content, TableauRequestID: content.TableauRequestID,
	}, nil
}
