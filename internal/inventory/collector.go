package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corecache "github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

const inventoryRefreshWarningHelp = "The live result is complete, but the cache was not updated; retry the live command to refresh it."

type Result struct {
	Entries     []corecache.ResourceEntry
	filtered    bool
	published   corecache.ReplaceResult
	requestIDs  []string
	cacheErr    error
	skippedRows int
	kind        string
}

func (i Result) FinalRequestID() string { return finalRequestID(i.requestIDs) }

func Collect(ctx context.Context, executor tableaucache.Executor, store *corecache.Store, scope tableaucache.Scope, environment, site string, observedAt time.Time, options ...Options) (Result, error) {
	config := tableaucache.Config{}
	option := Options{}
	if len(options) > 0 {
		option = options[0]
	}
	config.MaxConcurrency = option.MaxConcurrency
	engine, err := tableaucache.NewEngine(executor, config)
	if err != nil {
		return Result{}, err
	}
	snapshot, err := engine.CollectInventory(ctx, scope, tableaucache.InventoryOptions{SkipMalformedRecords: true, Filter: option.Filter, MaxRows: 10000})
	if err != nil {
		return Result{}, err
	}
	entries, skippedRows, err := resourceEntries(snapshot, environment, site, observedAt)
	if err != nil {
		return Result{}, err
	}
	sort.Slice(entries, func(i, j int) bool {
		leftName, rightName := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		leftProject, rightProject := strings.ToLower(entries[i].ProjectPath), strings.ToLower(entries[j].ProjectPath)
		if leftProject != rightProject {
			return leftProject < rightProject
		}
		return entries[i].LUID < entries[j].LUID
	})
	inventory := Result{
		filtered: option.Filter != "",
		Entries:  entries, requestIDs: append([]string(nil), snapshot.TableauRequestIDs...),
		skippedRows: skippedRows, kind: inventoryKind(scope),
	}
	if skippedRows > 0 {
		inventory.cacheErr = errors.New("incomplete live inventory cannot replace a complete cache scope")
		return inventory, nil
	}
	if len(entries) > 10000 {
		return Result{}, errors.New("--all exceeds the 10000-record bound; use narrower filters")
	}
	if option.Filter != "" {
		inventory.cacheErr = store.UpsertResources(ctx, entries)
		return inventory, nil
	}
	result, err := store.ReplaceResourceScope(ctx, corecache.ResourceScopeReplacement{
		Environment: environment, Site: site, Kind: inventoryKind(scope), Source: "tableau-rest", GeneratedAt: observedAt, Entries: entries,
	})
	inventory.published, inventory.cacheErr = result, err
	return inventory, nil
}

func (i Result) warningSource(observedAt time.Time) *readsource.Metadata {
	if i.skippedRows == 0 {
		return liveInventoryWarningSource(observedAt)
	}
	value := readsource.Live(observedAt)
	value.Coverage = readsource.CoveragePartial
	value.CoverageReason = "malformed_records_skipped"
	value.CacheWarning = i.incompleteWarning()
	return &value
}

func (i Result) warningHelp() string {
	if i.skippedRows == 0 {
		return inventoryRefreshWarningHelp
	}
	return i.incompleteWarning() + " Retry the live command or use narrower filters for the available records."
}

func (i Result) SourceAndHelp(observedAt time.Time, now func() time.Time) (*readsource.Metadata, string) {
	if i.cacheErr != nil {
		return i.warningSource(observedAt), i.warningHelp()
	}
	if i.filtered {
		value := readsource.Live(now().UTC())
		return &value, ""
	}
	return liveSource(observedAt, i.published.GenerationID), ""
}

func (i Result) incompleteWarning() string {
	record := i.kind + " record"
	if i.skippedRows != 1 {
		record += "s"
	}
	return fmt.Sprintf("The live inventory skipped %d malformed %s, so coverage is incomplete and the local cache snapshot was not updated.", i.skippedRows, record)
}

func resourceEntries(snapshot tableaucache.InventorySnapshot, environment, site string, observedAt time.Time) ([]corecache.ResourceEntry, int, error) {
	projects, err := projects(snapshot, true)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]corecache.ResourceEntry, 0, len(snapshot.Rows))
	skippedRows := snapshot.SkippedRows
	for _, row := range snapshot.Rows {
		entry, err := inventoryResourceEntry(snapshot.Scope, row, projects, environment, site, observedAt)
		if err != nil {
			skippedRows++
			continue
		}
		entries = append(entries, entry)
	}
	return entries, skippedRows, nil
}

type inventoryProject struct {
	name   string
	parent string
	path   string
}

func projects(snapshot tableaucache.InventorySnapshot, tolerant ...bool) (map[string]inventoryProject, error) {
	skipMalformed := len(tolerant) > 0 && tolerant[0]
	hasProjects := snapshot.Scope == tableaucache.ScopeProjects
	var rows [][]any
	if snapshot.Scope == tableaucache.ScopeProjects {
		rows = snapshot.Rows
	}
	{
		for _, dependency := range snapshot.Dependencies {
			if dependency.Scope == tableaucache.ScopeProjects {
				rows = dependency.Rows
				hasProjects = true
				break
			}
		}
	}
	if !hasProjects && (snapshot.Scope == tableaucache.ScopeWorkbooks || snapshot.Scope == tableaucache.ScopeDatasources || snapshot.Scope == tableaucache.ScopeFlows) {
		return nil, errors.New("complete content inventory omitted project dependencies")
	}
	projects := make(map[string]inventoryProject, len(rows))
	for _, row := range rows {
		if len(row) < 3 {
			if skipMalformed {
				continue
			}
			return nil, errors.New("project inventory row is incomplete")
		}
		id, idOK := row[0].(string)
		name, nameOK := row[1].(string)
		parent, parentOK := row[2].(string)
		if !idOK || !nameOK || !parentOK || strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" {
			if skipMalformed {
				continue
			}
			return nil, errors.New("project inventory returned incomplete authoritative identity")
		}
		// A slash in a display name does not invalidate authoritative LUIDs.
		// Path selection resolves ambiguity separately from inventory collection.
		projects[id] = inventoryProject{name: name, parent: parent}
	}
	state := make(map[string]uint8, len(projects))
	var resolve func(string) (string, error)
	resolve = func(id string) (string, error) {
		project, ok := projects[id]
		if !ok {
			return "", fmt.Errorf("project inventory omitted referenced project %q", id)
		}
		switch state[id] {
		case 1:
			return "", fmt.Errorf("project inventory contains a hierarchy cycle at %q", id)
		case 2:
			return project.path, nil
		}
		state[id] = 1
		path := project.name
		if project.parent != "" {
			parentPath, err := resolve(project.parent)
			if err != nil {
				return "", err
			}
			path = parentPath + "/" + project.name
		}
		project.path = path
		projects[id] = project
		state[id] = 2
		return path, nil
	}
	for id := range projects {
		if _, err := resolve(id); err != nil {
			if skipMalformed {
				continue
			}
			return nil, err
		}
	}
	return projects, nil
}

func inventoryResourceEntry(scope tableaucache.Scope, row []any, projects map[string]inventoryProject, environment, site string, observedAt time.Time) (corecache.ResourceEntry, error) {
	text := func(index int) (string, error) {
		if index >= len(row) {
			return "", errors.New("inventory row is incomplete")
		}
		if row[index] == nil {
			return "", nil
		}
		value, ok := row[index].(string)
		if !ok {
			return "", fmt.Errorf("inventory column %d is not text", index)
		}
		return value, nil
	}
	texts := func(indices ...int) ([]string, error) {
		values := make([]string, len(indices))
		for index, column := range indices {
			value, err := text(column)
			if err != nil {
				return nil, err
			}
			values[index] = value
		}
		return values, nil
	}
	payloadMap := func(index int) (map[string]any, error) {
		encoded, err := text(index)
		if err != nil {
			return nil, err
		}
		value := make(map[string]any)
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil, fmt.Errorf("decode complete inventory projection: %w", err)
		}
		return value, nil
	}
	id, err := text(0)
	if err != nil {
		return corecache.ResourceEntry{}, err
	}
	name, err := text(1)
	if err != nil {
		return corecache.ResourceEntry{}, err
	}
	entry := corecache.ResourceEntry{Environment: environment, Site: site, Kind: inventoryKind(scope), LUID: id, Name: name, Coverage: "summary", ObservedAt: observedAt}
	var payload map[string]any
	switch scope {
	case tableaucache.ScopeWorkbooks:
		values, err := texts(2, 3, 5)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		projectID, ownerID, updatedAt := values[0], values[1], values[2]
		project, ok := projects[projectID]
		if !ok || project.path == "" {
			return corecache.ResourceEntry{}, fmt.Errorf("workbook %q references unknown project %q", id, projectID)
		}
		entry.ProjectPath, entry.Owner = project.path, ownerID
		entry.ProjectLUID = projectID
		payload, err = payloadMap(6)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["project_luid"], payload["project_path"], payload["owner_luid"], payload["updated_at"] = id, name, projectID, project.path, ownerID, updatedAt
	case tableaucache.ScopeDatasources:
		values, err := texts(2, 3, 4)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		projectID, ownerID, updatedAt := values[0], values[1], values[2]
		project, ok := projects[projectID]
		if !ok || project.path == "" {
			return corecache.ResourceEntry{}, fmt.Errorf("datasource %q references unknown project %q", id, projectID)
		}
		entry.ProjectPath, entry.Owner = project.path, ownerID
		entry.ProjectLUID = projectID
		payload, err = payloadMap(5)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["project_luid"], payload["project_name"], payload["project_path"], payload["owner_luid"], payload["updated_at"] = id, name, projectID, project.name, project.path, ownerID, updatedAt
	case tableaucache.ScopeFlows:
		values, err := texts(2, 3, 4, 5)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		projectID, ownerID, fileType, updatedAt := values[0], values[1], values[2], values[3]
		project, ok := projects[projectID]
		if !ok || project.path == "" {
			return corecache.ResourceEntry{}, fmt.Errorf("flow %q references unknown project %q", id, projectID)
		}
		entry.ProjectPath, entry.Owner = project.path, ownerID
		entry.ProjectLUID = projectID
		payload, err = payloadMap(6)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["project_luid"], payload["project_name"], payload["project_path"], payload["owner_luid"], payload["file_type"], payload["updated_at"] = id, name, projectID, project.name, project.path, ownerID, fileType, updatedAt
	case tableaucache.ScopeProjects:
		values, err := texts(2, 3, 4)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		parentID, description, ownerID := values[0], values[1], values[2]
		project := projects[id]
		if project.path == "" {
			return corecache.ResourceEntry{}, fmt.Errorf("project %q has no canonical hierarchy path", id)
		}
		topLevel := parentID == ""
		entry.ProjectPath, entry.Owner = project.path, ownerID
		payload, err = payloadMap(5)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["parent_luid"], payload["description"], payload["owner_luid"], payload["top_level"] = id, name, parentID, description, ownerID, topLevel
	case tableaucache.ScopeUsers:
		values, err := texts(2, 3, 4)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		email, siteRole, lastLogin := values[0], values[1], values[2]
		payload, err = payloadMap(5)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["email"], payload["site_role"], payload["last_login"] = id, name, email, siteRole, lastLogin
	case tableaucache.ScopeGroups:
		values, err := texts(2)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		domain := values[0]
		payload, err = payloadMap(3)
		if err != nil {
			return corecache.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["domain"] = id, name, domain
	default:
		return corecache.ResourceEntry{}, fmt.Errorf("unsupported complete inventory scope %q", scope)
	}
	entry.Payload, err = json.Marshal(payload)
	return entry, err
}

func inventoryKind(scope tableaucache.Scope) string {
	switch scope {
	case tableaucache.ScopeWorkbooks:
		return "workbook"
	case tableaucache.ScopeDatasources:
		return "datasource"
	case tableaucache.ScopeFlows:
		return "flow"
	case tableaucache.ScopeProjects:
		return "project"
	case tableaucache.ScopeUsers:
		return "user"
	case tableaucache.ScopeGroups:
		return "group"
	default:
		return ""
	}
}

func liveSource(observedAt time.Time, generationID string) *readsource.Metadata {
	value := readsource.LiveInventory(observedAt, generationID)
	return &value
}

func liveInventoryWarningSource(observedAt time.Time) *readsource.Metadata {
	value := readsource.LiveInventoryWarning(observedAt)
	return &value
}

func finalRequestID(requestIDs []string) string {
	if len(requestIDs) == 0 {
		return ""
	}
	return requestIDs[len(requestIDs)-1]
}

func RefreshError(operation, environment, site string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Retry the live list; the previous cache snapshot remains unchanged.")
	return &errs.Error{
		ID: operation + ".inventory_refresh_failed", Kind: errs.KindOperation, Operation: operation,
		Environment: environment, Site: site, Summary: "Complete live inventory refresh failed.", Cause: err,
		Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err),
	}
}

func ValidateAll(all bool, source *readsource.Metadata) error {
	if all && (source == nil || source.Coverage != readsource.CoverageComplete) {
		return errs.New(errs.KindRuntime, "--all requires complete inventory coverage; refresh the cache or retry the live list")
	}
	return nil
}

type Options struct {
	MaxConcurrency int
	Filter         string
}
