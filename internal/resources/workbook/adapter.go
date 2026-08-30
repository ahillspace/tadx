// Package workbook establishes the resource-adapter pattern for Tableau workbooks.
package workbook

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/ahillspace/tadx/internal/identity"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

const adapterPageSize = 1000

// Client is the narrow Tableau client family consumed by this adapter.
type Client interface {
	List(context.Context, int, int) (tableauworkbook.WorkbookPage, error)
	ListProjects(context.Context, int, int) (tableauworkbook.ProjectPage, error)
	Download(context.Context, string, *bool) (tableauworkbook.Download, error)
	Publish(context.Context, tableauworkbook.PublishRequest) (tableauworkbook.PublishResult, error)
}

// Workbook is the normalized resource identity.
type Workbook struct {
	LUID        string
	Name        string
	ContentURL  string
	ProjectLUID string
	ProjectPath string
	OwnerLUID   string
}

// Project is a normalized exact destination project.
type Project struct {
	LUID string
	Name string
	Path string
}

// Adapter isolates workbook-specific Tableau API behavior.
type Adapter struct{ client Client }

// NewAdapter creates the first resource adapter.
func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

// ResolveWorkbook applies LUID-authoritative exact selection across all pages.
func (a *Adapter) ResolveWorkbook(ctx context.Context, selector identity.Selector) (Workbook, error) {
	items, err := a.allWorkbooks(ctx)
	if err != nil {
		return Workbook{}, err
	}
	candidates := make([]identity.Candidate, len(items))
	byID := make(map[identity.LUID]Workbook, len(items))
	for index, item := range items {
		candidate := identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: item.ProjectName}
		candidates[index] = candidate
		byID[candidate.LUID] = Workbook{LUID: item.LUID, Name: item.Name, ContentURL: item.ContentURL, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectName, OwnerLUID: item.OwnerLUID}
	}
	resolved, err := identity.Resolve(selector, candidates)
	if err != nil {
		return Workbook{}, err
	}
	return byID[resolved.LUID], nil
}

// FindWorkbooks returns exact name and project matches for collision checks.
func (a *Adapter) FindWorkbooks(ctx context.Context, name, projectLUID string) ([]Workbook, error) {
	items, err := a.allWorkbooks(ctx)
	if err != nil {
		return nil, err
	}
	var matches []Workbook
	for _, item := range items {
		if item.Name == name && item.ProjectLUID == projectLUID {
			matches = append(matches, Workbook{LUID: item.LUID, Name: item.Name, ContentURL: item.ContentURL, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectName, OwnerLUID: item.OwnerLUID})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].LUID < matches[j].LUID })
	return matches, nil
}

// ResolveProject resolves a LUID or exact slash-delimited project path.
func (a *Adapter) ResolveProject(ctx context.Context, selector identity.Selector) (Project, error) {
	items, err := a.allProjects(ctx)
	if err != nil {
		return Project{}, err
	}
	byID := make(map[string]tableauworkbook.Project, len(items))
	for _, item := range items {
		byID[item.LUID] = item
	}
	paths := make(map[string]string, len(items))
	var buildPath func(string, map[string]bool) (string, error)
	buildPath = func(id string, visiting map[string]bool) (string, error) {
		if path, ok := paths[id]; ok {
			return path, nil
		}
		item, ok := byID[id]
		if !ok {
			return "", fmt.Errorf("project %q references missing parent", id)
		}
		if visiting[id] {
			return "", fmt.Errorf("project hierarchy contains a cycle at %q", id)
		}
		visiting[id] = true
		path := item.Name
		if item.ParentLUID != "" {
			parent, err := buildPath(item.ParentLUID, visiting)
			if err != nil {
				return "", err
			}
			path = parent + "/" + item.Name
		}
		delete(visiting, id)
		paths[id] = path
		return path, nil
	}
	candidates := make([]identity.Candidate, 0, len(items))
	projects := make(map[identity.LUID]Project, len(items))
	for _, item := range items {
		path, err := buildPath(item.LUID, make(map[string]bool))
		if err != nil {
			return Project{}, err
		}
		candidate := identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: path}
		candidates = append(candidates, candidate)
		projects[candidate.LUID] = Project{LUID: item.LUID, Name: item.Name, Path: path}
	}
	resolved, err := identity.Resolve(selector, candidates)
	if err != nil {
		return Project{}, err
	}
	return projects[resolved.LUID], nil
}

// DownloadWorkbook downloads one authoritative workbook.
func (a *Adapter) DownloadWorkbook(ctx context.Context, luid string, includeExtract *bool) (tableauworkbook.Download, error) {
	if luid == "" {
		return tableauworkbook.Download{}, errors.New("workbook LUID is required")
	}
	return a.client.Download(ctx, luid, includeExtract)
}

// PublishWorkbook publishes through the released workbook client family.
func (a *Adapter) PublishWorkbook(ctx context.Context, input tableauworkbook.PublishRequest) (tableauworkbook.PublishResult, error) {
	return a.client.Publish(ctx, input)
}

func (a *Adapter) allWorkbooks(ctx context.Context) ([]tableauworkbook.Workbook, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("workbook resource adapter is not configured")
	}
	var result []tableauworkbook.Workbook
	for number := 1; ; number++ {
		page, err := a.client.List(ctx, number, adapterPageSize)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if len(result) >= page.Page.Total || len(page.Items) == 0 {
			return result, nil
		}
	}
}

func (a *Adapter) allProjects(ctx context.Context) ([]tableauworkbook.Project, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("workbook resource adapter is not configured")
	}
	var result []tableauworkbook.Project
	for number := 1; ; number++ {
		page, err := a.client.ListProjects(ctx, number, adapterPageSize)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)
		if len(result) >= page.Page.Total || len(page.Items) == 0 {
			return result, nil
		}
	}
}
