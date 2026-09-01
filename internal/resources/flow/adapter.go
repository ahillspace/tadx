// Package flow applies authoritative identity rules to Tableau flow data.
package flow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
)

const resolutionPageSize = 1000

// Client is the narrow docs-only Tableau flow seam.
type Client interface {
	List(context.Context, tableauflow.ListRequest) (tableauflow.Page, error)
	Get(context.Context, string) (tableauflow.Flow, error)
	Download(context.Context, string) (tableauflow.Download, error)
}

// ProjectPathResolver supplies canonical hierarchy paths without a resource-package dependency.
type ProjectPathResolver interface {
	ResolveProjectPath(context.Context, string) (string, error)
}

// Adapter owns flow selection and native download validation.
type Adapter struct {
	client   Client
	projects ProjectPathResolver
}

// NewAdapter creates a flow resource adapter.
func NewAdapter(client Client, projects ProjectPathResolver) *Adapter {
	return &Adapter{client: client, projects: projects}
}

// Flow is one normalized authoritative flow.
type Flow struct {
	LUID, Name, Description, FileType, ProjectLUID, ProjectName, ProjectPath, OwnerLUID, CreatedAt, UpdatedAt string
	Tags                                                                                                      []string
	Parameters                                                                                                []tableauflow.Parameter
	OutputSteps                                                                                               []tableauflow.OutputStep
	RequestID                                                                                                 string
}

// Page is one bounded normalized page.
type Page struct {
	Number, Size, Total int
	Items               []Flow
	RequestID           string
}

// ListFlows returns one validated page.
func (a *Adapter) ListFlows(ctx context.Context, input tableauflow.ListRequest) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("flow resource adapter is not configured")
	}
	page, err := a.client.List(ctx, input)
	if err != nil {
		return Page{}, err
	}
	if err := validatePage(page, input.PageNumber, input.PageSize); err != nil {
		return Page{}, err
	}
	items := make([]Flow, len(page.Items))
	seen := make(map[string]tableauflow.Flow, len(page.Items))
	for index, item := range page.Items {
		if err := recordFlow(seen, item); err != nil {
			return Page{}, err
		}
		items[index] = normalize(item, "")
	}
	return Page{Number: page.Number, Size: page.Size, Total: page.Total, Items: items, RequestID: page.TableauRequestID}, nil
}

// ResolveFlow resolves one authoritative LUID or exact name and project path.
func (a *Adapter) ResolveFlow(ctx context.Context, selector identity.Selector) (Flow, error) {
	if a == nil || a.client == nil || a.projects == nil {
		return Flow{}, errors.New("flow resource adapter is not configured")
	}
	if selector.LUID != "" {
		item, err := a.client.Get(ctx, string(selector.LUID))
		if err != nil {
			return Flow{}, err
		}
		if item.LUID != string(selector.LUID) {
			return Flow{}, fmt.Errorf("flow response returned LUID %q, expected %q", item.LUID, selector.LUID)
		}
		if err := validateIdentity(item); err != nil {
			return Flow{}, err
		}
		path, err := a.projects.ResolveProjectPath(ctx, item.ProjectLUID)
		if err != nil {
			return Flow{}, err
		}
		return normalize(item, path), nil
	}
	if strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "" {
		return Flow{}, errors.New("flow selection requires a LUID or exact name and project path")
	}
	byLUID := make(map[string]Flow)
	seen := make(map[string]tableauflow.Flow)
	for number := 1; number <= 1000; number++ {
		page, err := a.client.List(ctx, tableauflow.ListRequest{PageNumber: number, PageSize: resolutionPageSize, Name: selector.Name})
		if err != nil {
			return Flow{}, err
		}
		if err := validatePage(page, number, resolutionPageSize); err != nil {
			return Flow{}, err
		}
		for _, item := range page.Items {
			if item.Name != selector.Name {
				continue
			}
			if err := recordFlow(seen, item); err != nil {
				return Flow{}, err
			}
			path, err := a.projects.ResolveProjectPath(ctx, item.ProjectLUID)
			if err != nil {
				return Flow{}, err
			}
			if path == selector.ProjectPath {
				byLUID[item.LUID] = normalize(item, path)
			}
		}
		if number*page.Size >= page.Total {
			break
		}
	}
	items := make([]Flow, 0, len(byLUID))
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
		return Flow{}, err
	}
	return byLUID[string(resolved.LUID)], nil
}

// DownloadFlow returns unchanged TFL or TFLX bytes.
func (a *Adapter) DownloadFlow(ctx context.Context, luid string) (tableauflow.Download, error) {
	if a == nil || a.client == nil || strings.TrimSpace(luid) == "" {
		return tableauflow.Download{}, errors.New("flow LUID and configured client are required")
	}
	download, err := a.client.Download(ctx, luid)
	if err != nil {
		return tableauflow.Download{}, err
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(download.Filename)))
	if extension != ".tfl" && extension != ".tflx" {
		return tableauflow.Download{}, fmt.Errorf("flow download returned unsupported filename %q", download.Filename)
	}
	return download, nil
}

// FindFlows returns case-insensitive name collisions scoped to one project LUID.
func (a *Adapter) FindFlows(ctx context.Context, name, projectLUID string) ([]Flow, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("flow resource adapter is not configured")
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(projectLUID) == "" {
		return nil, errors.New("flow collision lookup requires name and project LUID")
	}
	seen := make(map[string]tableauflow.Flow)
	matches := make(map[string]Flow)
	for number := 1; number <= 1000; number++ {
		page, err := a.client.List(ctx, tableauflow.ListRequest{PageNumber: number, PageSize: resolutionPageSize, Name: name, ProjectLUID: projectLUID})
		if err != nil {
			return nil, err
		}
		if err := validatePage(page, number, resolutionPageSize); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if err := recordFlow(seen, item); err != nil {
				return nil, err
			}
			if strings.EqualFold(item.Name, name) && item.ProjectLUID == projectLUID {
				matches[item.LUID] = normalize(item, "")
			}
		}
		if number*page.Size >= page.Total {
			result := make([]Flow, 0, len(matches))
			for _, item := range matches {
				result = append(result, item)
			}
			sort.Slice(result, func(i, j int) bool { return result[i].LUID < result[j].LUID })
			return result, nil
		}
	}
	return nil, errors.New("flow collision scan exceeded the 1000-page bound")
}

func validateIdentity(item tableauflow.Flow) error {
	if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.ProjectLUID) == "" {
		return errors.New("flow response omitted an authoritative LUID, name, or project LUID")
	}
	return nil
}

func recordFlow(seen map[string]tableauflow.Flow, item tableauflow.Flow) error {
	if err := validateIdentity(item); err != nil {
		return err
	}
	if current, exists := seen[item.LUID]; exists && !equalFlow(current, item) {
		return fmt.Errorf("Tableau flow list returned conflicting records for LUID %q", item.LUID)
	}
	seen[item.LUID] = item
	return nil
}

func equalFlow(left, right tableauflow.Flow) bool {
	return reflect.DeepEqual(left, right)
}

func validatePage(page tableauflow.Page, number, size int) error {
	if number <= 0 || size <= 0 || page.Number != number || page.Size <= 0 || page.Size > size || page.Total < 0 || len(page.Items) > page.Size || (page.Number-1)*page.Size+len(page.Items) > page.Total {
		return errors.New("flow list returned inconsistent pagination")
	}
	return nil
}

func normalize(item tableauflow.Flow, path string) Flow {
	return Flow{LUID: item.LUID, Name: item.Name, Description: item.Description, FileType: item.FileType, ProjectLUID: item.ProjectLUID, ProjectName: item.ProjectName, ProjectPath: path, OwnerLUID: item.OwnerLUID, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Tags: item.Tags, Parameters: item.Parameters, OutputSteps: item.OutputSteps, RequestID: item.TableauRequestID}
}
