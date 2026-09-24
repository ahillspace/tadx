// Package workbook establishes the resource-adapter pattern for Tableau workbooks.
package workbook

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

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

// InventoryClient is the filtered workbook list seam.
type InventoryClient interface {
	ListWorkbooks(context.Context, tableauworkbook.ListRequest) (tableauworkbook.WorkbookPage, error)
}

// MutationClient is the exact workbook mutation seam.
type MutationClient interface {
	Delete(context.Context, string) (tableauworkbook.MutationResult, error)
}

type updateClient interface {
	Update(context.Context, tableauworkbook.UpdateRequest) (tableauworkbook.MutationResult, error)
}

// UpdateWorkbook changes explicit fields on one exact authoritative workbook.
func (a *Adapter) UpdateWorkbook(ctx context.Context, input tableauworkbook.UpdateRequest) (tableauworkbook.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(input.LUID) == "" {
		return tableauworkbook.MutationResult{}, errors.New("workbook LUID and configured client are required")
	}
	client, ok := a.client.(updateClient)
	if !ok {
		return tableauworkbook.MutationResult{}, errors.New("workbook mutation client is not configured")
	}
	return client.Update(ctx, input)
}

// ProjectPathResolver supplies canonical hierarchy paths without a resource-package dependency.
type ProjectPathResolver interface {
	ResolveProjectPath(context.Context, string) (string, error)
}

type projectPathBatchResolver interface {
	ResolveProjectPaths(context.Context, []string) (map[string]string, error)
}

type projectPathValidator interface {
	ValidateProjectPath(context.Context, string) error
}

// Workbook is the normalized resource identity.
type Workbook struct {
	LUID, Name, ContentURL, ProjectLUID, ProjectPath, OwnerLUID string
	Description, CreatedAt, UpdatedAt, RequestID                string
	Tags                                                        []string
}

// Page is one bounded normalized workbook page.
type Page struct {
	Number, Size, Total int
	Items               []Workbook
	RequestID           string
}

// Project is a normalized exact destination project.
type Project struct {
	LUID string
	Name string
	Path string
}

// Adapter isolates workbook-specific Tableau API behavior.
type Adapter struct {
	client   Client
	projects ProjectPathResolver
}

// NewAdapter creates the first resource adapter.
func NewAdapter(client Client) *Adapter { return &Adapter{client: client} }

// NewAdapterWithProjectResolver uses the shared canonical project hierarchy resolver.
func NewAdapterWithProjectResolver(client Client, projects ProjectPathResolver) *Adapter {
	return &Adapter{client: client, projects: projects}
}

// ListWorkbooks returns one validated, bounded page.
func (a *Adapter) ListWorkbooks(ctx context.Context, input tableauworkbook.ListRequest) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("workbook resource adapter is not configured")
	}
	if input.PageNumber <= 0 {
		return Page{}, errors.New("workbook page number must be positive")
	}
	if input.PageSize <= 0 || input.PageSize > adapterPageSize {
		return Page{}, fmt.Errorf("workbook page size must be between 1 and %d", adapterPageSize)
	}
	if input.ProjectLUID != "" {
		return a.listWorkbooksByProjectLUID(ctx, input)
	}
	return a.listWorkbooksPage(ctx, input)
}

func (a *Adapter) listWorkbooksPage(ctx context.Context, input tableauworkbook.ListRequest) (Page, error) {
	client, ok := a.client.(InventoryClient)
	if !ok {
		return Page{}, errors.New("workbook inventory client is not configured")
	}
	page, err := client.ListWorkbooks(ctx, input)
	if err != nil {
		return Page{}, err
	}
	if err := validateWorkbookPage(page, input.PageNumber, input.PageSize); err != nil {
		return Page{}, err
	}
	items, err := a.normalizeWorkbookItems(ctx, page.Items)
	if err != nil {
		return Page{}, err
	}
	return Page{Number: page.Page.Number, Size: page.Page.Size, Total: page.Page.Total, Items: items, RequestID: page.TableauRequestID}, nil
}

// CollectProjectWorkbooks obtains one validated project-filtered inventory for
// a command's --all result. It retains no state between calls or commands.
func (a *Adapter) CollectProjectWorkbooks(ctx context.Context, input tableauworkbook.ListRequest) (Page, error) {
	if a == nil || a.client == nil || input.ProjectLUID == "" {
		return Page{}, errors.New("project LUID and configured workbook adapter are required")
	}
	const maximumRows = 10000
	input.PageNumber, input.PageSize = 1, maximumRows
	page, err := a.listWorkbooksByProjectLUID(ctx, input)
	if err != nil {
		return Page{}, err
	}
	if page.Total > maximumRows {
		return Page{}, errors.New("--all exceeds the 10000-record bound; use narrower filters")
	}
	return page, nil
}

func (a *Adapter) listWorkbooksByProjectLUID(ctx context.Context, input tableauworkbook.ListRequest) (Page, error) {
	client, ok := a.client.(InventoryClient)
	if !ok {
		return Page{}, errors.New("workbook inventory client is not configured")
	}
	request := input
	request.PageSize = adapterPageSize
	seen := make(map[string]tableauworkbook.Workbook)
	matches := make([]tableauworkbook.Workbook, 0)
	expectedTotal, expectedSize := -1, -1
	requestID := ""
	for number := 1; number <= 1000; number++ {
		request.PageNumber = number
		page, err := client.ListWorkbooks(ctx, request)
		if err != nil {
			return Page{}, err
		}
		if err := validateWorkbookPage(page, number, adapterPageSize); err != nil {
			return Page{}, err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Page.Total, page.Page.Size
		} else if page.Page.Total != expectedTotal {
			return Page{}, fmt.Errorf("workbook project filter pagination total changed from %d to %d", expectedTotal, page.Page.Total)
		} else if page.Page.Size != expectedSize {
			return Page{}, fmt.Errorf("workbook project filter pagination size changed from %d to %d", expectedSize, page.Page.Size)
		}
		requestID = page.TableauRequestID
		for _, item := range page.Items {
			if item.LUID == "" {
				return Page{}, fmt.Errorf("workbook %q omitted its authoritative LUID", item.Name)
			}
			if item.ProjectLUID == "" {
				return Page{}, fmt.Errorf("workbook %q with LUID %q omitted its authoritative project LUID", item.Name, item.LUID)
			}
			_, alreadySeen := seen[item.LUID]
			if err := recordWorkbookIdentity(seen, item); err != nil {
				return Page{}, err
			}
			if !alreadySeen && item.ProjectLUID == input.ProjectLUID &&
				(input.Name == "" || item.Name == input.Name) &&
				(input.ProjectName == "" || item.ProjectName == input.ProjectName) {
				matches = append(matches, item)
			}
		}
		offset := (page.Page.Number-1)*page.Page.Size + len(page.Items)
		if offset == page.Page.Total {
			break
		}
		if len(page.Items) == 0 {
			return Page{}, errors.New("workbook project filter pagination ended before the reported total")
		}
		if number == 1000 {
			return Page{}, errors.New("workbook project filter exceeded the bounded page limit")
		}
	}
	if expectedTotal < 0 {
		return Page{}, errors.New("workbook project filter returned no page")
	}
	start := (input.PageNumber - 1) * input.PageSize
	if start > len(matches) {
		return Page{}, fmt.Errorf("workbook project filter page %d exceeds the filtered total %d", input.PageNumber, len(matches))
	}
	end := min(start+input.PageSize, len(matches))
	items, err := a.normalizeWorkbookItems(ctx, matches[start:end])
	if err != nil {
		return Page{}, err
	}
	return Page{Number: input.PageNumber, Size: input.PageSize, Total: len(matches), Items: items, RequestID: requestID}, nil
}

func (a *Adapter) normalizeWorkbookItems(ctx context.Context, rawItems []tableauworkbook.Workbook) ([]Workbook, error) {
	items := make([]Workbook, len(rawItems))
	seen := make(map[string]tableauworkbook.Workbook, len(rawItems))
	var legacyPaths *projectPathIndex
	var resolvedPaths map[string]string
	var err error
	if a.projects == nil && len(rawItems) > 0 {
		projects, err := a.allProjects(ctx)
		if err != nil {
			return nil, err
		}
		legacyPaths = newProjectPathIndex(projects)
	} else if resolver, ok := a.projects.(projectPathBatchResolver); ok && len(rawItems) > 0 {
		projectLUIDs := make([]string, 0, len(rawItems))
		for _, item := range rawItems {
			projectLUIDs = append(projectLUIDs, item.ProjectLUID)
		}
		resolvedPaths, err = resolver.ResolveProjectPaths(ctx, projectLUIDs)
		if err != nil {
			return nil, err
		}
	}
	for index, item := range rawItems {
		if item.LUID == "" {
			return nil, fmt.Errorf("workbook %q omitted its authoritative LUID", item.Name)
		}
		if item.ProjectLUID == "" {
			return nil, fmt.Errorf("workbook %q with LUID %q omitted its authoritative project LUID", item.Name, item.LUID)
		}
		if err := recordWorkbookIdentity(seen, item); err != nil {
			return nil, err
		}
		path := resolvedPaths[item.ProjectLUID]
		if resolvedPaths == nil {
			path, err = a.resolveProjectPath(ctx, item.ProjectLUID, legacyPaths)
			if err != nil {
				return nil, err
			}
		} else if path == "" {
			return nil, fmt.Errorf("project hierarchy omitted workbook project LUID %q", item.ProjectLUID)
		}
		items[index] = normalizeWorkbook(item, path)
	}
	return items, nil
}

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
		if item.ProjectLUID == "" {
			return Workbook{}, fmt.Errorf("workbook %q with LUID %q omitted its authoritative project LUID", item.Name, item.LUID)
		}
		workbook := normalizeWorkbook(item, item.ProjectName)
		return a.resolveSelectedProjectPath(ctx, workbook)
	}

	var paths *projectPathIndex
	if selector.ProjectPath != "" {
		if validator, ok := a.projects.(projectPathValidator); ok {
			if err := validator.ValidateProjectPath(ctx, selector.ProjectPath); err != nil {
				return Workbook{}, err
			}
		}
	}
	if selector.ProjectPath != "" && a.projects == nil {
		projects, err := a.allProjects(ctx)
		if err != nil {
			return Workbook{}, err
		}
		paths = newProjectPathIndex(projects)
		candidates := make([]identity.Candidate, 0, len(projects))
		for _, project := range projects {
			path, err := paths.path(project.LUID, make(map[string]bool))
			if err != nil {
				return Workbook{}, err
			}
			candidates = append(candidates, identity.Candidate{LUID: identity.LUID(project.LUID), ProjectPath: path})
		}
		if _, err := identity.Resolve(identity.Selector{ProjectPath: selector.ProjectPath}, candidates); err != nil {
			return Workbook{}, err
		}
	}
	seenByLUID := make(map[string]tableauworkbook.Workbook)
	itemsByLUID := make(map[string]tableauworkbook.Workbook, 2)
	err := a.scanWorkbooks(ctx, tableauworkbook.ListRequest{Name: selector.Name, ProjectLUID: string(selector.ProjectLUID)}, func(item tableauworkbook.Workbook) error {
		if selector.Name != "" && item.Name != selector.Name {
			return nil
		}
		if item.LUID == "" {
			return fmt.Errorf("matching workbook %q omitted its authoritative LUID", item.Name)
		}
		if item.ProjectLUID == "" {
			return fmt.Errorf("matching workbook %q with LUID %q omitted its authoritative project LUID", item.Name, item.LUID)
		}
		if selector.ProjectLUID != "" && item.ProjectLUID != string(selector.ProjectLUID) {
			return nil
		}
		if err := recordWorkbookIdentity(seenByLUID, item); err != nil {
			return err
		}
		if selector.ProjectPath != "" {
			projectPath := item.ProjectName
			if item.ProjectLUID != "" {
				path, err := a.resolveProjectPath(ctx, item.ProjectLUID, paths)
				if err != nil {
					return err
				}
				projectPath = path
			}
			if projectPath != selector.ProjectPath {
				return nil
			}
			item.ProjectName = projectPath
		}
		if _, exists := itemsByLUID[item.LUID]; !exists {
			itemsByLUID[item.LUID] = item
			if len(itemsByLUID) == 2 {
				return errWorkbookAmbiguityProven
			}
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

func (a *Adapter) resolveSelectedProjectPath(ctx context.Context, workbook Workbook) (Workbook, error) {
	if a.projects != nil {
		var err error
		workbook.ProjectPath, err = a.projects.ResolveProjectPath(ctx, workbook.ProjectLUID)
		return workbook, err
	}
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
		candidate := identity.Candidate{LUID: identity.LUID(item.LUID), Name: item.Name, ProjectPath: projectPath, ProjectLUID: identity.LUID(item.ProjectLUID)}
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
	seenByLUID := make(map[string]tableauworkbook.Workbook)
	byLUID := make(map[string]Workbook)
	err := a.scanWorkbooks(ctx, tableauworkbook.ListRequest{Name: name}, func(item tableauworkbook.Workbook) error {
		if !strings.EqualFold(item.Name, name) {
			return nil
		}
		if item.LUID == "" {
			return fmt.Errorf("workbook %q omitted its authoritative LUID", name)
		}
		if item.ProjectLUID == "" {
			return fmt.Errorf("workbook %q with LUID %q omitted its authoritative project LUID", name, item.LUID)
		}
		if err := recordWorkbookIdentity(seenByLUID, item); err != nil {
			return err
		}
		if item.ProjectLUID != projectLUID {
			return nil
		}
		if _, exists := byLUID[item.LUID]; !exists {
			byLUID[item.LUID] = normalizeWorkbook(item, item.ProjectName)
			if len(byLUID) == 2 {
				return errWorkbookAmbiguityProven
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

func recordWorkbookIdentity(byLUID map[string]tableauworkbook.Workbook, item tableauworkbook.Workbook) error {
	current, exists := byLUID[item.LUID]
	if !exists {
		byLUID[item.LUID] = item
		return nil
	}
	current.TableauRequestID = ""
	item.TableauRequestID = ""
	if !reflect.DeepEqual(current, item) {
		return fmt.Errorf("Tableau workbook list returned conflicting records for LUID %q", item.LUID)
	}
	return nil
}

// ResolveProject resolves a LUID or exact slash-delimited project path.
func (a *Adapter) ResolveProject(ctx context.Context, selector identity.Selector) (Project, error) {
	if resolver, ok := a.projects.(interface {
		ResolveProject(context.Context, identity.Selector) (Project, error)
	}); ok {
		return resolver.ResolveProject(ctx, selector)
	}
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

// BeginProjectResolution forwards an explicit validation phase when configured.
func (a *Adapter) BeginProjectResolution(ctx context.Context) context.Context {
	if a != nil {
		if resolver, ok := a.projects.(interface {
			BeginProjectResolution(context.Context) context.Context
		}); ok {
			return resolver.BeginProjectResolution(ctx)
		}
	}
	return ctx
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

// DeleteWorkbook removes one exact authoritative workbook LUID.
func (a *Adapter) DeleteWorkbook(ctx context.Context, luid string) (tableauworkbook.MutationResult, error) {
	if a == nil || a.client == nil || strings.TrimSpace(luid) == "" {
		return tableauworkbook.MutationResult{}, errors.New("workbook LUID and configured client are required")
	}
	client, ok := a.client.(MutationClient)
	if !ok {
		return tableauworkbook.MutationResult{}, errors.New("workbook mutation client is not configured")
	}
	return client.Delete(ctx, luid)
}

func (a *Adapter) scanWorkbooks(ctx context.Context, request tableauworkbook.ListRequest, visit func(tableauworkbook.Workbook) error) error {
	if a == nil || a.client == nil {
		return errors.New("workbook resource adapter is not configured")
	}
	seen := 0
	expectedTotal, expectedSize := -1, -1
	for number := 1; number <= 1000; number++ {
		request.PageNumber, request.PageSize = number, adapterPageSize
		var page tableauworkbook.WorkbookPage
		var err error
		if inventory, ok := a.client.(InventoryClient); ok {
			page, err = inventory.ListWorkbooks(ctx, request)
		} else {
			page, err = a.client.List(ctx, number, adapterPageSize)
		}
		if err != nil {
			return err
		}
		if err := validateWorkbookPage(page, number, adapterPageSize); err != nil {
			return err
		}
		if expectedTotal < 0 {
			expectedTotal, expectedSize = page.Page.Total, page.Page.Size
		} else if page.Page.Total != expectedTotal {
			return fmt.Errorf("workbook pagination total changed from %d to %d", expectedTotal, page.Page.Total)
		} else if page.Page.Size != expectedSize {
			return fmt.Errorf("workbook pagination size changed from %d to %d", expectedSize, page.Page.Size)
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
	return errors.New("workbook resolution exceeded the bounded page limit")
}

func normalizeWorkbook(item tableauworkbook.Workbook, projectPath string) Workbook {
	return Workbook{LUID: item.LUID, Name: item.Name, ContentURL: item.ContentURL, ProjectLUID: item.ProjectLUID, ProjectPath: projectPath, OwnerLUID: item.OwnerLUID, Description: item.Description, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Tags: append([]string(nil), item.Tags...), RequestID: item.TableauRequestID}
}

func (a *Adapter) resolveProjectPath(ctx context.Context, projectLUID string, paths *projectPathIndex) (string, error) {
	if a.projects != nil {
		return a.projects.ResolveProjectPath(ctx, projectLUID)
	}
	if paths == nil {
		return "", errors.New("workbook project hierarchy is not configured")
	}
	return paths.path(projectLUID, make(map[string]bool))
}

func validateWorkbookPage(page tableauworkbook.WorkbookPage, number, size int) error {
	if number <= 0 || size <= 0 || page.Page.Number != number || page.Page.Size <= 0 || page.Page.Size > size || page.Page.Total < 0 || len(page.Items) > page.Page.Size || (page.Page.Number-1)*page.Page.Size+len(page.Items) > page.Page.Total {
		return errors.New("workbook list returned inconsistent pagination")
	}
	return nil
}

func (a *Adapter) allProjects(ctx context.Context) ([]tableauworkbook.Project, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("workbook resource adapter is not configured")
	}
	byLUID := make(map[string]tableauworkbook.Project)
	seen := 0
	for number := 1; ; number++ {
		page, err := a.client.ListProjects(ctx, number, adapterPageSize)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if item.LUID == "" {
				return nil, fmt.Errorf("project %q omitted its authoritative LUID", item.Name)
			}
			current, exists := byLUID[item.LUID]
			if exists && current != item {
				return nil, fmt.Errorf("Tableau project list returned conflicting records for LUID %q", item.LUID)
			}
			if !exists {
				byLUID[item.LUID] = item
			}
		}
		seen += len(page.Items)
		if seen >= page.Page.Total || len(page.Items) == 0 {
			result := make([]tableauworkbook.Project, 0, len(byLUID))
			for _, item := range byLUID {
				result = append(result, item)
			}
			sort.Slice(result, func(left, right int) bool { return result[left].LUID < result[right].LUID })
			return result, nil
		}
	}
}
