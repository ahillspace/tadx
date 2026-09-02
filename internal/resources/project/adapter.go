// Package project resolves exact Tableau project identities without HTTP behavior.
package project

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
)

const resolutionPageSize = 1000

// Client is the narrow docs-only Tableau project seam.
type Client interface {
	List(context.Context, tableauproject.ListRequest) (tableauproject.Page, error)
}

// ListRequest selects one bounded project page.
type ListRequest = tableauproject.ListRequest

// Project is one normalized authoritative project.
type Project struct {
	LUID                            string
	Name                            string
	Path                            string
	Description                     string
	ParentLUID                      string
	OwnerLUID                       string
	TopLevel                        *bool
	ContentPermissions              string
	ControllingPermissionsProjectID string
	CreatedAt                       string
	UpdatedAt                       string
	ProjectCount                    *int
	WorkbookCount                   *int
	ViewCount                       *int
	DatasourceCount                 *int
	RequestID                       string
}

// Page is one bounded normalized project page.
type Page struct {
	Number    int
	Size      int
	Total     int
	Items     []Project
	RequestID string
}

// Adapter isolates project identity rules from actions.
type Adapter struct{ client Client }

// NewAdapter creates a project resource adapter.
func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

// ListProjects returns exactly one validated upstream page.
func (a *Adapter) ListProjects(ctx context.Context, input ListRequest) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("project resource adapter is not configured")
	}
	upstream, err := a.client.List(ctx, input)
	if err != nil {
		return Page{}, err
	}
	if err := validatePage(upstream, input.PageNumber, input.PageSize); err != nil {
		return Page{}, err
	}
	items := make([]Project, len(upstream.Items))
	seen := make(map[string]tableauproject.Project, len(upstream.Items))
	for index, item := range upstream.Items {
		if err := recordProject(seen, item); err != nil {
			return Page{}, err
		}
		items[index] = normalize(item, "")
	}
	return Page{Number: upstream.Number, Size: upstream.Size, Total: upstream.Total, Items: items, RequestID: upstream.TableauRequestID}, nil
}

// ResolveProject resolves one LUID or exact slash-delimited path.
func (a *Adapter) ResolveProject(ctx context.Context, selector identity.Selector) (Project, error) {
	if selector.LUID == "" && strings.TrimSpace(selector.ProjectPath) == "" {
		return Project{}, errors.New("project LUID or exact project path is required")
	}
	items, requestID, err := a.all(ctx)
	if err != nil {
		return Project{}, err
	}
	index := newPathIndex(items)
	candidates := make([]identity.Candidate, 0, len(items))
	byLUID := make(map[identity.LUID]Project, len(items))
	for _, item := range items {
		path, pathErr := index.path(item.LUID, make(map[string]bool))
		if pathErr != nil {
			return Project{}, pathErr
		}
		candidate := identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: path}
		candidates = append(candidates, candidate)
		project := normalize(item, path)
		project.RequestID = requestID
		byLUID[candidate.LUID] = project
	}
	resolved, err := identity.Resolve(selector, candidates)
	if err != nil {
		return Project{}, err
	}
	return byLUID[resolved.LUID], nil
}

// ResolveProjectPath returns the canonical hierarchy path for one authoritative LUID.
func (a *Adapter) ResolveProjectPath(ctx context.Context, luid string) (string, error) {
	project, err := a.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(luid)})
	return project.Path, err
}

// FindProjectCollisions returns case-insensitive sibling-name collisions.
func (a *Adapter) FindProjectCollisions(ctx context.Context, name, parentLUID string) ([]Project, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("project collision check requires a name")
	}
	items, requestID, err := a.all(ctx)
	if err != nil {
		return nil, err
	}
	index := newPathIndex(items)
	matches := make([]Project, 0, 1)
	for _, item := range items {
		if !strings.EqualFold(item.Name, name) || item.ParentLUID != parentLUID {
			continue
		}
		path, pathErr := index.path(item.LUID, make(map[string]bool))
		if pathErr != nil {
			return nil, pathErr
		}
		project := normalize(item, path)
		project.RequestID = requestID
		matches = append(matches, project)
	}
	sort.Slice(matches, func(left, right int) bool { return matches[left].LUID < matches[right].LUID })
	return matches, nil
}

// NormalizeMutationProject builds a canonical path from the authoritative
// mutation response without waiting for the new identity to enter the search index.
func (a *Adapter) NormalizeMutationProject(ctx context.Context, item tableauproject.Project) (Project, error) {
	item.LUID = strings.TrimSpace(item.LUID)
	item.Name = strings.TrimSpace(item.Name)
	if item.LUID == "" || item.Name == "" {
		return Project{}, errors.New("project mutation returned an incomplete authoritative identity")
	}
	if strings.Contains(item.Name, "/") {
		return Project{}, fmt.Errorf("Tableau project %q has a name containing %q, which is not addressable by an exact project path", item.LUID, "/")
	}
	path := item.Name
	if item.ParentLUID != "" {
		parentPath, err := a.ResolveProjectPath(ctx, item.ParentLUID)
		if err != nil {
			return Project{}, fmt.Errorf("resolve project mutation parent %q: %w", item.ParentLUID, err)
		}
		path = parentPath + "/" + item.Name
	}
	return normalize(item, path), nil
}

func (a *Adapter) all(ctx context.Context) ([]tableauproject.Project, string, error) {
	if a == nil || a.client == nil {
		return nil, "", errors.New("project resource adapter is not configured")
	}
	byLUID := make(map[string]tableauproject.Project)
	requestID := ""
	expectedTotal, expectedSize := -1, -1
	for pageNumber := 1; pageNumber <= 1000; pageNumber++ {
		page, err := a.client.List(ctx, tableauproject.ListRequest{PageNumber: pageNumber, PageSize: resolutionPageSize})
		if err != nil {
			return nil, "", err
		}
		requestID = page.TableauRequestID
		if err := validatePage(page, pageNumber, resolutionPageSize); err != nil {
			return nil, "", err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Total, page.Size
		} else if page.Total != expectedTotal {
			return nil, "", fmt.Errorf("project pagination total changed from %d to %d", expectedTotal, page.Total)
		} else if page.Size != expectedSize {
			return nil, "", fmt.Errorf("project pagination size changed from %d to %d", expectedSize, page.Size)
		}
		for _, item := range page.Items {
			if err := recordProject(byLUID, item); err != nil {
				return nil, "", err
			}
		}
		if pageNumber*page.Size >= page.Total {
			items := make([]tableauproject.Project, 0, len(byLUID))
			for _, item := range byLUID {
				items = append(items, item)
			}
			sort.Slice(items, func(i, j int) bool { return items[i].LUID < items[j].LUID })
			return items, requestID, nil
		}
	}
	return nil, "", errors.New("project hierarchy exceeded the 1000-page resolution bound")
}

func validatePage(page tableauproject.Page, requestedNumber, requestedSize int) error {
	if requestedNumber <= 0 || requestedSize <= 0 {
		return errors.New("project page number and size must be positive")
	}
	if page.Number != requestedNumber || page.Size <= 0 || page.Size > requestedSize || page.Total < 0 {
		return fmt.Errorf("project list returned inconsistent pagination number=%d size=%d total=%d", page.Number, page.Size, page.Total)
	}
	offset := (page.Number - 1) * page.Size
	if offset > page.Total || len(page.Items) > page.Size || offset+len(page.Items) > page.Total {
		return fmt.Errorf("project list returned inconsistent page item count %d", len(page.Items))
	}
	return nil
}

func recordProject(items map[string]tableauproject.Project, item tableauproject.Project) error {
	item.LUID = strings.TrimSpace(item.LUID)
	item.Name = strings.TrimSpace(item.Name)
	if item.LUID == "" || item.Name == "" {
		return errors.New("project list returned an incomplete authoritative identity")
	}
	// Project paths are slash-delimited, so a name containing "/" would make the
	// hierarchy path ambiguous (parent "A" with child "B/C" is indistinguishable
	// from parent "A/B" with child "C"). Reject it at the authoritative boundary
	// rather than risk resolving an exact project path to the wrong project.
	if strings.Contains(item.Name, "/") {
		return fmt.Errorf("Tableau project %q has a name containing %q, which is not addressable by an exact project path", item.LUID, "/")
	}
	if current, exists := items[item.LUID]; exists && current != item {
		return fmt.Errorf("Tableau project list returned conflicting records for LUID %q", item.LUID)
	}
	items[item.LUID] = item
	return nil
}

type pathIndex struct {
	byID  map[string]tableauproject.Project
	paths map[string]string
}

func newPathIndex(items []tableauproject.Project) *pathIndex {
	index := &pathIndex{byID: make(map[string]tableauproject.Project, len(items)), paths: make(map[string]string, len(items))}
	for _, item := range items {
		index.byID[item.LUID] = item
	}
	return index
}

func (i *pathIndex) path(luid string, visiting map[string]bool) (string, error) {
	if path, exists := i.paths[luid]; exists {
		return path, nil
	}
	item, exists := i.byID[luid]
	if !exists {
		return "", fmt.Errorf("project %q references a missing parent", luid)
	}
	if visiting[luid] {
		return "", fmt.Errorf("project hierarchy contains a cycle at %q", luid)
	}
	visiting[luid] = true
	path := item.Name
	if item.ParentLUID != "" {
		parent, err := i.path(item.ParentLUID, visiting)
		if err != nil {
			return "", err
		}
		path = parent + "/" + item.Name
	}
	delete(visiting, luid)
	i.paths[luid] = path
	return path, nil
}

func normalize(item tableauproject.Project, path string) Project {
	return Project{
		LUID: item.LUID, Name: item.Name, Path: path, Description: item.Description,
		ParentLUID: item.ParentLUID, OwnerLUID: item.OwnerLUID, TopLevel: item.TopLevel,
		ContentPermissions: item.ContentPermissions, ControllingPermissionsProjectID: item.ControllingPermissionsProjectID,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ProjectCount: item.ProjectCount,
		WorkbookCount: item.WorkbookCount, ViewCount: item.ViewCount, DatasourceCount: item.DatasourceCount,
	}
}
