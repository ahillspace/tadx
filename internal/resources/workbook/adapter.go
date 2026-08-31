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

var errWorkbookAmbiguityProven = errors.New("workbook ambiguity proven")

// Client is the narrow Tableau client family consumed by this adapter.
type Client interface {
	Get(context.Context, string) (tableauworkbook.Workbook, error)
	List(context.Context, int, int) (tableauworkbook.WorkbookPage, error)
	ListProjects(context.Context, int, int) (tableauworkbook.ProjectPage, error)
	Download(context.Context, string, *bool) (tableauworkbook.Download, error)
	Prepare(context.Context, tableauworkbook.PublishRequest) (*tableauworkbook.PreparedPublish, error)
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
	if a == nil || a.client == nil {
		return Workbook{}, errors.New("workbook resource adapter is not configured")
	}
	if selector.LUID != "" {
		item, err := a.client.Get(ctx, string(selector.LUID))
		if err != nil {
			return Workbook{}, err
		}
		if item.LUID == "" || item.LUID != string(selector.LUID) {
			return Workbook{}, fmt.Errorf("workbook response returned authoritative LUID %q, expected %q", item.LUID, selector.LUID)
		}
		workbook := normalizeWorkbook(item, item.ProjectName)
		if workbook.ProjectLUID == "" {
			return workbook, nil
		}
		return a.resolveSelectedProjectPath(ctx, workbook)
	}

	var paths *projectPathIndex
	if selector.ProjectPath != "" {
		projects, err := a.allProjects(ctx)
		if err != nil {
			return Workbook{}, err
		}
		paths = newProjectPathIndex(projects)
	}
	itemsByLUID := make(map[string]tableauworkbook.Workbook, 2)
	err := a.scanWorkbooks(ctx, func(item tableauworkbook.Workbook) error {
		if selector.Name != "" && item.Name != selector.Name {
			return nil
		}
		if selector.ProjectPath != "" {
			projectPath := item.ProjectName
			if item.ProjectLUID != "" {
				path, err := paths.path(item.ProjectLUID, make(map[string]bool))
				if err != nil {
					return err
				}
				projectPath = path
			}
			if projectPath != selector.ProjectPath {
				return nil
			}
		}
		if item.LUID == "" {
			return fmt.Errorf("matching workbook %q omitted its authoritative LUID", item.Name)
		}
		current, exists := itemsByLUID[item.LUID]
		if !exists || workbookItemLess(item, current) {
			itemsByLUID[item.LUID] = item
		}
		if !exists && len(itemsByLUID) == 2 {
			return errWorkbookAmbiguityProven
		}
		return nil
	})
	if err != nil && !errors.Is(err, errWorkbookAmbiguityProven) {
		return Workbook{}, err
	}
	items := make([]tableauworkbook.Workbook, 0, len(itemsByLUID))
	for _, item := range itemsByLUID {
		items = append(items, item)
	}
	workbook, err := resolveWorkbook(selector, items, paths)
	if err != nil || workbook.ProjectLUID == "" || paths != nil {
		return workbook, err
	}
	return a.resolveSelectedProjectPath(ctx, workbook)
}

func workbookItemLess(left, right tableauworkbook.Workbook) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	return left.ProjectName < right.ProjectName
}

func (a *Adapter) resolveSelectedProjectPath(ctx context.Context, workbook Workbook) (Workbook, error) {
	projects, err := a.allProjects(ctx)
	if err != nil {
		return Workbook{}, err
	}
	workbook.ProjectPath, err = newProjectPathIndex(projects).path(workbook.ProjectLUID, make(map[string]bool))
	if err != nil {
		return Workbook{}, err
	}
	return workbook, nil
}

func resolveWorkbook(selector identity.Selector, items []tableauworkbook.Workbook, paths *projectPathIndex) (Workbook, error) {
	candidates := make([]identity.Candidate, len(items))
	byCandidate := make(map[identity.Candidate]Workbook, len(items))
	for index, item := range items {
		projectPath := item.ProjectName
		if paths != nil && item.ProjectLUID != "" && (selector.Name == "" || item.Name == selector.Name) {
			resolvedPath, err := paths.path(item.ProjectLUID, make(map[string]bool))
			if err != nil {
				return Workbook{}, err
			}
			projectPath = resolvedPath
		}
		candidate := identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: projectPath}
		candidates[index] = candidate
		if _, exists := byCandidate[candidate]; !exists {
			byCandidate[candidate] = normalizeWorkbook(item, projectPath)
		}
	}
	resolved, err := identity.Resolve(selector, candidates)
	if err != nil {
		return Workbook{}, err
	}
	return byCandidate[resolved], nil
}

// FindWorkbooks returns exact name and project matches for collision checks.
func (a *Adapter) FindWorkbooks(ctx context.Context, name, projectLUID string) ([]Workbook, error) {
	byLUID := make(map[string]Workbook)
	err := a.scanWorkbooks(ctx, func(item tableauworkbook.Workbook) error {
		if item.Name == name && item.ProjectLUID == projectLUID {
			if item.LUID == "" {
				return fmt.Errorf("workbook %q in project %q omitted its authoritative LUID", name, projectLUID)
			}
			if _, exists := byLUID[item.LUID]; !exists {
				byLUID[item.LUID] = normalizeWorkbook(item, item.ProjectName)
				if len(byLUID) == 2 {
					return errWorkbookAmbiguityProven
				}
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errWorkbookAmbiguityProven) {
		return nil, err
	}
	matches := make([]Workbook, 0, len(byLUID))
	for _, workbook := range byLUID {
		matches = append(matches, workbook)
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
	paths := newProjectPathIndex(items)
	if selector.LUID != "" {
		item, ok := paths.byID[string(selector.LUID)]
		if !ok {
			_, err := identity.Resolve(selector, nil)
			return Project{}, err
		}
		path, err := paths.path(item.LUID, make(map[string]bool))
		if err != nil {
			return Project{}, err
		}
		return Project{LUID: item.LUID, Name: item.Name, Path: path}, nil
	}
	candidates := make([]identity.Candidate, 0, len(items))
	projects := make(map[identity.LUID]Project, len(items))
	for _, item := range items {
		path, err := paths.path(item.LUID, make(map[string]bool))
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

type projectPathIndex struct {
	byID  map[string]tableauworkbook.Project
	paths map[string]string
}

func newProjectPathIndex(items []tableauworkbook.Project) *projectPathIndex {
	index := &projectPathIndex{byID: make(map[string]tableauworkbook.Project, len(items)), paths: make(map[string]string, len(items))}
	for _, item := range items {
		index.byID[item.LUID] = item
	}
	return index
}

func (i *projectPathIndex) path(id string, visiting map[string]bool) (string, error) {
	if path, ok := i.paths[id]; ok {
		return path, nil
	}
	item, ok := i.byID[id]
	if !ok {
		return "", fmt.Errorf("project %q references missing parent", id)
	}
	if visiting[id] {
		return "", fmt.Errorf("project hierarchy contains a cycle at %q", id)
	}
	visiting[id] = true
	path := item.Name
	if item.ParentLUID != "" {
		parent, err := i.path(item.ParentLUID, visiting)
		if err != nil {
			return "", err
		}
		path = parent + "/" + item.Name
	}
	delete(visiting, id)
	i.paths[id] = path
	return path, nil
}

// DownloadWorkbook downloads one authoritative workbook.
func (a *Adapter) DownloadWorkbook(ctx context.Context, luid string, includeExtract *bool) (tableauworkbook.Download, error) {
	if luid == "" {
		return tableauworkbook.Download{}, errors.New("workbook LUID is required")
	}
	return a.client.Download(ctx, luid, includeExtract)
}

// PrepareWorkbook uploads and validates content before the final publish request.
func (a *Adapter) PrepareWorkbook(ctx context.Context, input tableauworkbook.PublishRequest) (*tableauworkbook.PreparedPublish, error) {
	return a.client.Prepare(ctx, input)
}

func (a *Adapter) scanWorkbooks(ctx context.Context, visit func(tableauworkbook.Workbook) error) error {
	if a == nil || a.client == nil {
		return errors.New("workbook resource adapter is not configured")
	}
	seen := 0
	for number := 1; ; number++ {
		page, err := a.client.List(ctx, number, adapterPageSize)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			if err := visit(item); err != nil {
				return err
			}
		}
		seen += len(page.Items)
		if seen >= page.Page.Total || len(page.Items) == 0 {
			return nil
		}
	}
}

func normalizeWorkbook(item tableauworkbook.Workbook, projectPath string) Workbook {
	return Workbook{LUID: item.LUID, Name: item.Name, ContentURL: item.ContentURL, ProjectLUID: item.ProjectLUID, ProjectPath: projectPath, OwnerLUID: item.OwnerLUID}
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
		for _, item := range page.Items {
			if item.LUID == "" {
				return nil, fmt.Errorf("project %q omitted its authoritative LUID", item.Name)
			}
			result = append(result, item)
		}
		if len(result) >= page.Page.Total || len(page.Items) == 0 {
			return result, nil
		}
	}
}
