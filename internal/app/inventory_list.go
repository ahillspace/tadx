package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
)

const inventoryRefreshHelp = "Complete live inventory refreshed the local catalog; continuation reads use this snapshot."
const inventoryRefreshWarningHelp = "The live result is complete, but the catalog was not updated; retry without a continuation cursor to refresh it."

type collectedResourceInventory struct {
	entries     []corecatalog.ResourceEntry
	published   corecatalog.ReplaceResult
	requestIDs  []string
	catalogErr  error
	skippedRows int
	kind        string
	snapshotID  string
	snapshotErr error
}

func collectResourceInventory(ctx context.Context, executor tableaucatalog.Executor, store *corecatalog.Store, scope tableaucatalog.Scope, environment, site string, observedAt time.Time) (collectedResourceInventory, error) {
	engine, err := tableaucatalog.NewEngine(executor, tableaucatalog.Config{})
	if err != nil {
		return collectedResourceInventory{}, err
	}
	snapshot, err := engine.CollectInventory(ctx, scope, tableaucatalog.InventoryOptions{SkipMalformedRecords: true})
	if err != nil {
		return collectedResourceInventory{}, err
	}
	entries, skippedRows, err := inventoryResourceEntries(snapshot, environment, site, observedAt)
	if err != nil {
		return collectedResourceInventory{}, err
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
	inventory := collectedResourceInventory{
		entries: entries, requestIDs: append([]string(nil), snapshot.TableauRequestIDs...),
		skippedRows: skippedRows, kind: inventoryKind(scope),
	}
	if skippedRows > 0 {
		inventory.catalogErr = errors.New("incomplete live inventory cannot replace a complete catalog scope")
		inventory.snapshotID, inventory.snapshotErr = store.SavePartialInventory(ctx, corecatalog.ResourceScopeReplacement{
			Environment: environment, Site: site, Kind: inventory.kind, GeneratedAt: observedAt, Entries: entries,
		}, inventory.incompleteWarning())
		return inventory, nil
	}
	result, err := store.ReplaceResourceScope(ctx, corecatalog.ResourceScopeReplacement{
		Environment: environment, Site: site, Kind: inventoryKind(scope), Source: "tableau-rest", GeneratedAt: observedAt, Entries: entries,
	})
	inventory.published, inventory.catalogErr = result, err
	return inventory, nil
}

func (i collectedResourceInventory) warningSource(observedAt time.Time) *readsource.Metadata {
	if i.skippedRows == 0 {
		return liveInventoryWarningSource(observedAt)
	}
	value := readsource.Live(observedAt)
	value.Coverage = readsource.CoveragePartial
	value.CatalogWarning = i.incompleteWarning()
	return &value
}

func (i collectedResourceInventory) warningHelp() string {
	if i.skippedRows == 0 {
		return inventoryRefreshWarningHelp
	}
	if i.snapshotErr != nil {
		return i.incompleteWarning() + " Temporary snapshot storage failed; continuation is unavailable."
	}
	return i.incompleteWarning() + " Temporary continuation expires after 24 hours or earlier if its snapshot is evicted."
}

func (i collectedResourceInventory) incompleteWarning() string {
	record := i.kind + " record"
	if i.skippedRows != 1 {
		record += "s"
	}
	return fmt.Sprintf("The live inventory skipped %d malformed %s, so coverage is incomplete and the local catalog snapshot was not updated.", i.skippedRows, record)
}

type inventoryMemoryReader struct {
	entries    []corecatalog.ResourceEntry
	requestID  string
	snapshotID string
}

func (i collectedResourceInventory) memoryReader() inventoryMemoryReader {
	return inventoryMemoryReader{entries: i.entries, requestID: finalRequestID(i.requestIDs), snapshotID: i.snapshotID}
}

func (r inventoryMemoryReader) nextCursor(size int) string {
	if r.snapshotID == "" || size >= len(r.entries) {
		return ""
	}
	entry := r.entries[0]
	return corecatalog.PartialInventoryCursor(r.snapshotID, corecatalog.ResourceQuery{Environment: entry.Environment, Site: entry.Site, Kind: entry.Kind, Limit: size, Offset: size})
}

func (r inventoryMemoryReader) page(size int) []corecatalog.ResourceEntry {
	if size > len(r.entries) {
		size = len(r.entries)
	}
	return r.entries[:size]
}

func decodeInventoryPage[T any](entries []corecatalog.ResourceEntry) ([]T, error) {
	items := make([]T, len(entries))
	for index, entry := range entries {
		if err := json.Unmarshal(entry.Payload, &items[index]); err != nil {
			return nil, fmt.Errorf("decode live inventory row: %w", err)
		}
	}
	return items, nil
}

func (r inventoryMemoryReader) ListWorkbooks(_ context.Context, input workbooklist.PageRequest) (workbooklist.Page, error) {
	items, err := decodeInventoryPage[workbooklist.Workbook](r.page(input.PageSize))
	return workbooklist.Page{Number: 1, Size: input.PageSize, Total: len(r.entries), Workbooks: items, RequestID: r.requestID, SnapshotCursor: r.nextCursor(input.PageSize), SuppressContinuation: r.snapshotID == ""}, err
}

func (r inventoryMemoryReader) ListDatasources(_ context.Context, input datasourcelist.PageRequest) (datasourcelist.Page, error) {
	items, err := decodeInventoryPage[datasourcelist.Datasource](r.page(input.PageSize))
	return datasourcelist.Page{Number: 1, Size: input.PageSize, Total: len(r.entries), Datasources: items, RequestID: r.requestID, SnapshotCursor: r.nextCursor(input.PageSize), SuppressContinuation: r.snapshotID == ""}, err
}

func (r inventoryMemoryReader) ListFlows(_ context.Context, input flowlist.PageRequest) (flowlist.Page, error) {
	items, err := decodeInventoryPage[flowlist.Flow](r.page(input.PageSize))
	return flowlist.Page{Number: 1, Size: input.PageSize, Total: len(r.entries), Flows: items, RequestID: r.requestID, SnapshotCursor: r.nextCursor(input.PageSize), SuppressContinuation: r.snapshotID == ""}, err
}

func (r inventoryMemoryReader) ListProjects(_ context.Context, input projectlist.PageRequest) (projectlist.Page, error) {
	items, err := decodeInventoryPage[projectlist.Project](r.page(input.PageSize))
	return projectlist.Page{Number: 1, Size: input.PageSize, Total: len(r.entries), Projects: items, RequestID: r.requestID, SnapshotCursor: r.nextCursor(input.PageSize), SuppressContinuation: r.snapshotID == ""}, err
}

func (r inventoryMemoryReader) ListUsers(_ context.Context, input userlist.PageRequest) (userlist.Page, error) {
	items, err := decodeInventoryPage[userlist.User](r.page(input.PageSize))
	return userlist.Page{Number: 1, Size: input.PageSize, Total: len(r.entries), Users: items, RequestID: r.requestID, SnapshotCursor: r.nextCursor(input.PageSize), SuppressContinuation: r.snapshotID == ""}, err
}

func (r inventoryMemoryReader) ListGroups(_ context.Context, input grouplist.PageRequest) (grouplist.Page, error) {
	items, err := decodeInventoryPage[grouplist.Group](r.page(input.PageSize))
	return grouplist.Page{Number: 1, Size: input.PageSize, Total: len(r.entries), Groups: items, RequestID: r.requestID, SnapshotCursor: r.nextCursor(input.PageSize), SuppressContinuation: r.snapshotID == ""}, err
}

func inventoryResourceEntries(snapshot tableaucatalog.InventorySnapshot, environment, site string, observedAt time.Time) ([]corecatalog.ResourceEntry, int, error) {
	projects, err := inventoryProjects(snapshot, true)
	if err != nil {
		return nil, 0, err
	}
	entries := make([]corecatalog.ResourceEntry, 0, len(snapshot.Rows))
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

func inventoryProjects(snapshot tableaucatalog.InventorySnapshot, tolerant ...bool) (map[string]inventoryProject, error) {
	skipMalformed := len(tolerant) > 0 && tolerant[0]
	hasProjects := snapshot.Scope == tableaucatalog.ScopeProjects
	var rows [][]any
	if snapshot.Scope == tableaucatalog.ScopeProjects {
		rows = snapshot.Rows
	} else {
		for _, dependency := range snapshot.Dependencies {
			if dependency.Scope == tableaucatalog.ScopeProjects {
				rows = dependency.Rows
				hasProjects = true
				break
			}
		}
	}
	if !hasProjects && (snapshot.Scope == tableaucatalog.ScopeWorkbooks || snapshot.Scope == tableaucatalog.ScopeDatasources || snapshot.Scope == tableaucatalog.ScopeFlows) {
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
		if strings.Contains(name, "/") {
			if skipMalformed {
				continue
			}
			return nil, fmt.Errorf("Tableau project %q has a name containing %q, which is not addressable by an exact project path", id, "/")
		}
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

func inventoryResourceEntry(scope tableaucatalog.Scope, row []any, projects map[string]inventoryProject, environment, site string, observedAt time.Time) (corecatalog.ResourceEntry, error) {
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
		return corecatalog.ResourceEntry{}, err
	}
	name, err := text(1)
	if err != nil {
		return corecatalog.ResourceEntry{}, err
	}
	entry := corecatalog.ResourceEntry{Environment: environment, Site: site, Kind: inventoryKind(scope), LUID: id, Name: name, Coverage: "summary", ObservedAt: observedAt}
	var payload map[string]any
	switch scope {
	case tableaucatalog.ScopeWorkbooks:
		values, err := texts(2, 3, 5)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		projectID, ownerID, updatedAt := values[0], values[1], values[2]
		project, ok := projects[projectID]
		if !ok || project.path == "" {
			return corecatalog.ResourceEntry{}, fmt.Errorf("workbook %q references unknown project %q", id, projectID)
		}
		entry.ProjectPath, entry.Owner = project.path, ownerID
		payload, err = payloadMap(6)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["project_luid"], payload["project_path"], payload["owner_luid"], payload["updated_at"] = id, name, projectID, project.path, ownerID, updatedAt
	case tableaucatalog.ScopeDatasources:
		values, err := texts(2, 3, 4)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		projectID, ownerID, updatedAt := values[0], values[1], values[2]
		project, ok := projects[projectID]
		if !ok || project.path == "" {
			return corecatalog.ResourceEntry{}, fmt.Errorf("datasource %q references unknown project %q", id, projectID)
		}
		entry.ProjectPath, entry.Owner = project.path, ownerID
		payload, err = payloadMap(5)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["project_luid"], payload["project_name"], payload["project_path"], payload["owner_luid"], payload["updated_at"] = id, name, projectID, project.name, project.path, ownerID, updatedAt
	case tableaucatalog.ScopeFlows:
		values, err := texts(2, 3, 4, 5)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		projectID, ownerID, fileType, updatedAt := values[0], values[1], values[2], values[3]
		project, ok := projects[projectID]
		if !ok || project.path == "" {
			return corecatalog.ResourceEntry{}, fmt.Errorf("flow %q references unknown project %q", id, projectID)
		}
		entry.ProjectPath, entry.Owner = project.path, ownerID
		payload, err = payloadMap(6)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["project_luid"], payload["project_name"], payload["project_path"], payload["owner_luid"], payload["file_type"], payload["updated_at"] = id, name, projectID, project.name, project.path, ownerID, fileType, updatedAt
	case tableaucatalog.ScopeProjects:
		values, err := texts(2, 3, 4)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		parentID, description, ownerID := values[0], values[1], values[2]
		project := projects[id]
		if project.path == "" {
			return corecatalog.ResourceEntry{}, fmt.Errorf("project %q has no canonical hierarchy path", id)
		}
		topLevel := parentID == ""
		entry.ProjectPath, entry.Owner = project.path, ownerID
		payload, err = payloadMap(5)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["parent_luid"], payload["description"], payload["owner_luid"], payload["top_level"] = id, name, parentID, description, ownerID, topLevel
	case tableaucatalog.ScopeUsers:
		values, err := texts(2, 3, 4)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		email, siteRole, lastLogin := values[0], values[1], values[2]
		payload, err = payloadMap(5)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["email"], payload["site_role"], payload["last_login"] = id, name, email, siteRole, lastLogin
	case tableaucatalog.ScopeGroups:
		values, err := texts(2)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		domain := values[0]
		payload, err = payloadMap(3)
		if err != nil {
			return corecatalog.ResourceEntry{}, err
		}
		payload["luid"], payload["name"], payload["domain"] = id, name, domain
	default:
		return corecatalog.ResourceEntry{}, fmt.Errorf("unsupported complete inventory scope %q", scope)
	}
	entry.Payload, err = json.Marshal(payload)
	return entry, err
}

func inventoryKind(scope tableaucatalog.Scope) string {
	switch scope {
	case tableaucatalog.ScopeWorkbooks:
		return "workbook"
	case tableaucatalog.ScopeDatasources:
		return "datasource"
	case tableaucatalog.ScopeFlows:
		return "flow"
	case tableaucatalog.ScopeProjects:
		return "project"
	case tableaucatalog.ScopeUsers:
		return "user"
	case tableaucatalog.ScopeGroups:
		return "group"
	default:
		return ""
	}
}

func liveInventorySource(observedAt time.Time, generationID string) *readsource.Metadata {
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

func inventoryRefreshError(operation, environment, site string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Retry the live list; the previous catalog snapshot remains unchanged.")
	return &errs.Error{
		ID: operation + ".inventory_refresh_failed", Kind: errs.KindOperation, Operation: operation,
		Environment: environment, Site: site, Summary: "Complete live inventory refresh failed.", Cause: err,
		Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err),
	}
}
