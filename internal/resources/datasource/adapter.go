// Package datasource isolates published datasource REST behavior.
package datasource

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

// Client is the narrow Tableau datasource client used by inventory, identity resolution, and dependency acquisition.
type Client interface {
	Get(context.Context, string) (tableaudatasource.Datasource, error)
	List(context.Context, tableaudatasource.ListRequest) (tableaudatasource.Page, error)
	Download(context.Context, string, *bool) (tableaudatasource.Download, error)
}

// ProjectPathResolver supplies canonical project paths without coupling resource packages.
type ProjectPathResolver interface {
	ResolveProjectPath(context.Context, string) (string, error)
}

// Datasource is one normalized authoritative published datasource identity.
type Datasource struct {
	LUID                string
	Name                string
	ProjectLUID         string
	ProjectName         string
	ProjectPath         string
	Description         string
	Type                string
	ContentURL          string
	OwnerLUID           string
	CreatedAt           string
	UpdatedAt           string
	Size                *int64
	EncryptExtracts     *bool
	HasExtracts         *bool
	IsCertified         *bool
	CertificationNote   string
	UseRemoteQueryAgent *bool
	WebpageURL          string
	Tags                []string
	AskDataEnablement   string
	RequestID           string
}

// Page is one bounded normalized published datasource page.
type Page struct {
	Number    int
	Size      int
	Total     int
	Items     []Datasource
	RequestID string
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
type Adapter struct {
	client   Client
	projects ProjectPathResolver
}

// NewAdapter creates a datasource resource adapter.
func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

// NewAdapterWithProjectResolver creates a datasource adapter that supports exact selection.
func NewAdapterWithProjectResolver(client Client, projects ProjectPathResolver) *Adapter {
	return &Adapter{client: client, projects: projects}
}

// ListDatasources returns exactly one validated upstream page.
func (a *Adapter) ListDatasources(ctx context.Context, input tableaudatasource.ListRequest) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("datasource resource adapter is not configured")
	}
	upstream, err := a.client.List(ctx, input)
	if err != nil {
		return Page{}, err
	}
	if err := validateDatasourcePage(upstream, input.PageNumber, input.PageSize); err != nil {
		return Page{}, err
	}
	items := make([]Datasource, len(upstream.Items))
	seen := make(map[string]tableaudatasource.Datasource, len(upstream.Items))
	for index, item := range upstream.Items {
		if err := recordDatasource(seen, item); err != nil {
			return Page{}, err
		}
		items[index] = normalizeDatasource(item, "")
	}
	return Page{Number: upstream.Number, Size: upstream.Size, Total: upstream.Total, Items: items, RequestID: upstream.TableauRequestID}, nil
}

// ResolveDatasource resolves one authoritative LUID or exact name and canonical project path.
func (a *Adapter) ResolveDatasource(ctx context.Context, selector identity.Selector) (Datasource, error) {
	if a == nil || a.client == nil || a.projects == nil {
		return Datasource{}, errors.New("datasource resource adapter is not configured for identity resolution")
	}
	if selector.LUID != "" {
		item, err := a.client.Get(ctx, string(selector.LUID))
		if err != nil {
			return Datasource{}, err
		}
		if strings.TrimSpace(item.LUID) != string(selector.LUID) {
			return Datasource{}, fmt.Errorf("datasource response returned LUID %q, expected %q", item.LUID, selector.LUID)
		}
		if err := validateDatasourceIdentity(item); err != nil {
			return Datasource{}, err
		}
		path, err := a.projects.ResolveProjectPath(ctx, item.ProjectLUID)
		if err != nil {
			return Datasource{}, err
		}
		return normalizeDatasource(item, path), nil
	}
	if strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "" {
		return Datasource{}, errors.New("datasource selection requires a LUID or exact name and project path")
	}

	byLUID := make(map[string]Datasource)
	seen := make(map[string]tableaudatasource.Datasource)
	const pageSize = 1000
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= 1000; number++ {
		page, err := a.client.List(ctx, tableaudatasource.ListRequest{PageNumber: number, PageSize: pageSize, Name: selector.Name})
		if err != nil {
			return Datasource{}, err
		}
		if err := validateDatasourcePage(page, number, pageSize); err != nil {
			return Datasource{}, err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Total, page.Size
		} else if page.Total != expectedTotal {
			return Datasource{}, fmt.Errorf("datasource pagination total changed from %d to %d", expectedTotal, page.Total)
		} else if page.Size != expectedSize {
			return Datasource{}, fmt.Errorf("datasource pagination size changed from %d to %d", expectedSize, page.Size)
		}
		for _, item := range page.Items {
			if err := recordDatasource(seen, item); err != nil {
				return Datasource{}, err
			}
			if item.Name != selector.Name {
				continue
			}
			path, err := a.projects.ResolveProjectPath(ctx, item.ProjectLUID)
			if err != nil {
				return Datasource{}, err
			}
			if path == selector.ProjectPath {
				normalized := normalizeDatasource(item, path)
				normalized.RequestID = page.TableauRequestID
				byLUID[item.LUID] = normalized
			}
		}
		offset := (page.Number-1)*page.Size + len(page.Items)
		if offset == page.Total {
			items := make([]Datasource, 0, len(byLUID))
			for _, item := range byLUID {
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool { return items[i].LUID < items[j].LUID })
			candidates := make([]identity.Candidate, len(items))
			for index, item := range items {
				candidates[index] = identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: item.ProjectPath}
			}
			resolved, err := identity.Resolve(selector, candidates)
			if err != nil {
				return Datasource{}, err
			}
			selected := byLUID[string(resolved.LUID)]
			selected.RequestID = page.TableauRequestID
			return selected, nil
		}
		if len(page.Items) == 0 {
			return Datasource{}, errors.New("datasource pagination ended before the reported total")
		}
	}
	return Datasource{}, errors.New("datasource resolution exceeded the 1000-page bound")
}

func validateDatasourceIdentity(item tableaudatasource.Datasource) error {
	if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ProjectLUID) == "" || strings.TrimSpace(item.ProjectName) == "" {
		return errors.New("datasource response omitted an authoritative LUID, name, project LUID, or project name")
	}
	return nil
}

func recordDatasource(seen map[string]tableaudatasource.Datasource, item tableaudatasource.Datasource) error {
	if err := validateDatasourceIdentity(item); err != nil {
		return err
	}
	if current, exists := seen[item.LUID]; exists && !reflect.DeepEqual(current, item) {
		return fmt.Errorf("Tableau datasource list returned conflicting records for LUID %q", item.LUID)
	}
	seen[item.LUID] = item
	return nil
}

func validateDatasourcePage(page tableaudatasource.Page, number, requestedSize int) error {
	if number <= 0 || requestedSize <= 0 || page.Number != number || page.Size <= 0 || page.Size > requestedSize || page.Total < 0 || len(page.Items) > page.Size {
		return errors.New("datasource list returned inconsistent pagination")
	}
	offset := int64(page.Number-1) * int64(page.Size)
	if offset > int64(page.Total) || offset+int64(len(page.Items)) > int64(page.Total) {
		return errors.New("datasource list returned inconsistent pagination")
	}
	return nil
}

func normalizeDatasource(item tableaudatasource.Datasource, path string) Datasource {
	return Datasource{
		LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: path,
		Description: item.Description, Type: item.Type, ContentURL: item.ContentURL, OwnerLUID: item.OwnerLUID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Size: item.Size, EncryptExtracts: item.EncryptExtracts,
		HasExtracts: item.HasExtracts, IsCertified: item.IsCertified, CertificationNote: item.CertificationNote,
		UseRemoteQueryAgent: item.UseRemoteQueryAgent, WebpageURL: item.WebpageURL, Tags: append([]string(nil), item.Tags...),
		AskDataEnablement: item.AskDataEnablement, RequestID: item.TableauRequestID,
	}
}

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
