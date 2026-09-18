package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	"github.com/ahillspace/tadx/internal/tableau/job"
)

const maxOperationReceiptChecks = 100

// inspectPublicationOperation reads one exact local operation record. A live
// worker is never authenticated or disturbed. A finished remote-pending run
// may perform one bounded exact read per unfinished saved receipt.
func (r *runtimeDependencies) inspectPublicationOperation(ctx context.Context, input jobinspect.Input) (jobinspect.Result, error) {
	store, err := publicationOperationStore(r.operationDirectory)
	if err != nil {
		return jobinspect.Result{}, err
	}
	record, err := store.Read(input.OperationID)
	if err != nil {
		return jobinspect.Result{}, err
	}
	if !nativeLongOperation(record.Operation) {
		return jobinspect.Result{}, fmt.Errorf("operation %q is not a supported native publish or download operation", record.ID)
	}
	alive, err := store.Alive(record.ID)
	if err != nil {
		return jobinspect.Result{}, err
	}
	view := operationView(record, alive)
	if view.Environment == "" || view.Site == "" {
		environment, site := r.savedPublicationTarget(record)
		if view.Environment == "" {
			view.Environment = environment
		}
		if view.Site == "" {
			view.Site = site
		}
	}
	if input.Environment != "" && view.Environment != "" && input.Environment != view.Environment {
		return jobinspect.Result{}, fmt.Errorf("operation %q belongs to environment %q, not %q", record.ID, view.Environment, input.Environment)
	}
	if input.Site != "" && view.Environment != "" && input.Site != view.Site {
		return jobinspect.Result{}, fmt.Errorf("operation %q belongs to site %q, not %q", record.ID, view.Site, input.Site)
	}
	result := jobinspect.Result{Environment: view.Environment, Site: view.Site, Operation: view}
	if record.Phase == operationrun.PhaseCompleted || record.Phase == operationrun.PhaseFailed {
		warnings, items := r.readSavedPublicationReceipts(ctx, record)
		if updated, updateWarnings := r.reconcilePublicationReceipts(ctx, record, items); updated != nil {
			record = *updated
			view = operationView(record, alive)
			view.Environment, view.Site = result.Environment, result.Site
			result.Operation = view
			warnings = append(warnings, updateWarnings...)
		} else {
			warnings = append(warnings, updateWarnings...)
		}
		view.Warnings = append(view.Warnings, warnings...)
		view.Items = items
		result.Warnings = append(result.Warnings, warnings...)
	}
	if (record.Phase == operationrun.PhaseRemotePending || record.Phase == operationrun.PhaseRunning) && !alive {
		warnings, items, environment, site := r.inspectPublicationReceipts(ctx, record)
		if view.Environment == "" {
			view.Environment = environment
			result.Environment = environment
		}
		if view.Site == "" {
			view.Site = site
			result.Site = site
		}
		if input.Environment != "" && environment != "" && input.Environment != environment {
			return jobinspect.Result{}, fmt.Errorf("operation %q belongs to environment %q, not %q", record.ID, environment, input.Environment)
		}
		if input.Site != "" && site != "" && input.Site != site {
			return jobinspect.Result{}, fmt.Errorf("operation %q belongs to site %q, not %q", record.ID, site, input.Site)
		}
		if record.Phase == operationrun.PhaseRemotePending {
			if updated, updateWarnings := r.reconcilePublicationReceipts(ctx, record, items); updated != nil {
				record = *updated
				view = operationView(record, alive)
				view.Environment, view.Site = result.Environment, result.Site
				result.Operation = view
				warnings = append(warnings, updateWarnings...)
			} else {
				warnings = append(warnings, updateWarnings...)
			}
		}
		view.Warnings = append(view.Warnings, warnings...)
		view.Items = items
		result.Warnings = append(result.Warnings, warnings...)
	}
	return result, nil
}

func (r *runtimeDependencies) readSavedPublicationReceipts(ctx context.Context, record operationrun.Record) ([]string, []jobinspect.OperationItem) {
	receiptStore, err := r.publicationReceiptStore()
	if err != nil {
		return []string{err.Error()}, nil
	}
	warnings := make([]string, 0)
	items := make([]jobinspect.OperationItem, 0, min(len(record.ReceiptPaths), maxOperationReceiptChecks))
	connections := make(map[string]authenticatedTableau)
	for index, rawPath := range record.ReceiptPaths {
		if index >= maxOperationReceiptChecks {
			warnings = append(warnings, fmt.Sprintf("receipt display limited to %d of %d saved paths", maxOperationReceiptChecks, len(record.ReceiptPaths)))
			break
		}
		path := rawPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(receiptStore.Directory, path)
		}
		item := jobinspect.OperationItem{Key: receiptKey(index, path), ReceiptPath: filepath.ToSlash(path)}
		receipt, readErr := receiptStore.ReadPath(path)
		if readErr != nil {
			item.Status, item.Error = "unknown", readErr.Error()
			warnings = append(warnings, fmt.Sprintf("receipt %q could not be read: %v", filepath.ToSlash(path), readErr))
		} else {
			item.Name, item.Project = receipt.Name, receipt.ProjectID
			item.Status, item.ResourceID = receipt.Observation.Status, receipt.Observation.ResourceID
			item.JobID = receipt.Observation.ID
			item.Verification = receipt.Verification
			if item.Status == "succeeded" && receipt.Verification != "confirmed" {
				item, receipt, warnings = r.resolveReceiptItem(ctx, item, receipt, connections, warnings)
				if _, saveErr := receiptStore.Save(ctx, receipt); saveErr != nil {
					warnings = append(warnings, fmt.Sprintf("receipt %q was checked but its destination state could not be saved: %v", filepath.ToSlash(path), saveErr))
				}
			}
		}
		items = append(items, item)
	}
	return warnings, items
}

func (r *runtimeDependencies) savedPublicationTarget(record operationrun.Record) (string, string) {
	receiptStore, err := r.publicationReceiptStore()
	if err != nil {
		return "", ""
	}
	for index, rawPath := range record.ReceiptPaths {
		if index >= maxOperationReceiptChecks {
			break
		}
		path := rawPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(receiptStore.Directory, path)
		}
		receipt, readErr := receiptStore.ReadPath(path)
		if readErr == nil && (receipt.Environment != "" || receipt.Site != "") {
			return receipt.Environment, receipt.Site
		}
	}
	return "", ""
}

func operationView(record operationrun.Record, alive bool) *jobinspect.OperationView {
	status := operationStatus(record.Phase, alive)
	if record.Phase == operationrun.PhaseFailed && publicationSnapshotStatus(record) == "partial_failure" {
		status = "partial_failure"
	}
	stopped := record.Detached || (record.Phase == operationrun.PhaseRunning && !alive)
	view := &jobinspect.OperationView{
		ID:           record.ID,
		Operation:    record.Operation,
		Status:       status,
		Phase:        string(record.Phase),
		Alive:        alive,
		Detached:     record.Detached,
		Activity:     record.Activity,
		RequestedAt:  record.RequestedAt,
		StartedAt:    record.StartedAt,
		FinishedAt:   record.FinishedAt,
		WorkerPID:    record.WorkerPID,
		ExitCode:     record.ExitCode,
		Snapshot:     publicationSnapshot(record, false, stopped),
		FullSnapshot: publicationSnapshot(record, true, stopped),
		LiveResults:  append([]byte(nil), record.LiveResults...),
		ReceiptPaths: append([]string(nil), record.ReceiptPaths...),
	}
	view.Environment, view.Site = publicationTarget(record)
	if record.Phase != operationrun.PhaseCompleted || publicationNeedsDestination(record.FullResult) {
		view.CheckStatus = publicationCheckCommand(record)
		view.Details = []string{view.CheckStatus}
	}
	return view
}

func operationStatus(phase operationrun.Phase, alive bool) string {
	switch phase {
	case operationrun.PhaseRequested:
		return "starting"
	case operationrun.PhaseRunning:
		if !alive {
			return "interrupted"
		}
		return "running"
	case operationrun.PhaseRemotePending:
		return "pending"
	case operationrun.PhaseCompleted:
		return "succeeded"
	case operationrun.PhaseFailed:
		return "failed"
	default:
		return string(phase)
	}
}

func (r *runtimeDependencies) inspectPublicationReceipts(ctx context.Context, record operationrun.Record) ([]string, []jobinspect.OperationItem, string, string) {
	receiptStore, err := r.publicationReceiptStore()
	if err != nil {
		return []string{err.Error()}, nil, "", ""
	}
	warnings := make([]string, 0)
	items := make([]jobinspect.OperationItem, 0, min(len(record.ReceiptPaths), maxOperationReceiptChecks))
	environment, site := "", ""
	if len(record.ReceiptPaths) > maxOperationReceiptChecks {
		warnings = append(warnings, fmt.Sprintf("receipt checks limited to %d of %d saved paths", maxOperationReceiptChecks, len(record.ReceiptPaths)))
	}
	connections := make(map[string]authenticatedTableau)
	for index, rawPath := range record.ReceiptPaths {
		if index >= maxOperationReceiptChecks {
			break
		}
		path := rawPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(receiptStore.Directory, path)
		}
		receipt, readErr := receiptStore.ReadPath(path)
		item := jobinspect.OperationItem{Key: receiptKey(index, path), ReceiptPath: filepath.ToSlash(path)}
		if readErr != nil {
			item.Status = "unknown"
			item.Error = readErr.Error()
			warnings = append(warnings, fmt.Sprintf("receipt %q could not be read: %v", filepath.ToSlash(path), readErr))
			items = append(items, item)
			continue
		}
		item.Name, item.Project = receipt.Name, receipt.ProjectID
		item.JobID = receipt.Observation.ID
		if environment == "" {
			environment, site = receipt.Environment, receipt.Site
		}
		item.Status, item.ResourceID = receipt.Observation.Status, receipt.Observation.ResourceID
		if receipt.Observation.Terminal() {
			item, receipt, warnings = r.resolveReceiptItem(ctx, item, receipt, connections, warnings)
			if _, saveErr := receiptStore.Save(ctx, receipt); saveErr != nil {
				warnings = append(warnings, fmt.Sprintf("confirmed destination could not be saved: %v", saveErr))
			}
			items = append(items, item)
			continue
		}
		latest, checkErr := r.inspectReceiptOnce(ctx, receipt, connections)
		if checkErr != nil {
			item.Status = "unknown"
			item.Error = checkErr.Error()
			warnings = append(warnings, fmt.Sprintf("receipt %q was not rechecked: %v", filepath.ToSlash(path), checkErr))
			items = append(items, item)
			continue
		}
		item.Status, item.ResourceID = latest.Observation.Status, latest.Observation.ResourceID
		item, latest, warnings = r.resolveReceiptItem(ctx, item, latest, connections, warnings)
		if _, saveErr := receiptStore.Save(ctx, latest); saveErr != nil {
			warnings = append(warnings, fmt.Sprintf("receipt %q was checked but its state could not be saved: %v", filepath.ToSlash(path), saveErr))
		}
		items = append(items, item)
	}
	return warnings, items, environment, site
}

func (r *runtimeDependencies) resolveReceiptItem(ctx context.Context, item jobinspect.OperationItem, receipt jobmonitor.Receipt, connections map[string]authenticatedTableau, warnings []string) (jobinspect.OperationItem, jobmonitor.Receipt, []string) {
	if receipt.Observation.Status != "succeeded" {
		return item, receipt, warnings
	}
	item.Verification = receipt.Verification
	if receipt.Verification == "confirmed" || (item.ResourceID != "" && (receipt.Name == "" || receipt.ProjectID == "")) {
		return item, receipt, warnings
	}
	connection, err := r.receiptConnection(ctx, receipt, connections)
	if err != nil {
		item.Verification, receipt.Verification = "destination_unavailable", "destination_unavailable"
		warnings = append(warnings, fmt.Sprintf("destination for receipt %q could not be confirmed: %v", item.ReceiptPath, err))
		return item, receipt, warnings
	}
	resourceID, name, project, err := r.resolveReceiptDestination(ctx, receipt, connection)
	if err != nil {
		item.Verification, receipt.Verification = "destination_unavailable", "destination_unavailable"
		warnings = append(warnings, fmt.Sprintf("destination for receipt %q could not be confirmed: %v", item.ReceiptPath, err))
		return item, receipt, warnings
	}
	if resourceID != "" {
		if name != receipt.Name || project != receipt.ProjectID || (receipt.Observation.ResourceID != "" && resourceID != receipt.Observation.ResourceID) {
			item.Verification, receipt.Verification = "destination_mismatch", "destination_mismatch"
			warnings = append(warnings, "Published destination does not match the saved exact name and project; do not republish to repair confirmation.")
			return item, receipt, warnings
		}
		item.ResourceID = resourceID
		receipt.Observation.ResourceID = resourceID
	}
	if name != "" {
		item.Name = name
		receipt.Name = name
	}
	if project != "" {
		item.Project = project
		receipt.ProjectID = project
	}
	if item.ResourceID == "" {
		item.Verification, receipt.Verification = "destination_pending", "destination_pending"
	} else {
		item.Verification, receipt.Verification = "confirmed", "confirmed"
	}
	return item, receipt, warnings
}

func (r *runtimeDependencies) resolveReceiptDestination(ctx context.Context, receipt jobmonitor.Receipt, connection authenticatedTableau) (string, string, string, error) {
	clients := r.clients(connection)
	paths := r.discoveryPaths(connection)
	switch receipt.Operation {
	case "workbook.publish":
		adapter := resourceworkbook.NewAdapterWithProjectResolver(clients.workbooks, paths)
		if receipt.Observation.ResourceID != "" {
			item, err := adapter.ResolveWorkbook(ctx, identity.Selector{LUID: identity.LUID(receipt.Observation.ResourceID)})
			if err != nil {
				return receipt.Observation.ResourceID, receipt.Name, receipt.ProjectID, err
			}
			return item.LUID, item.Name, item.ProjectLUID, nil
		}
		if receipt.Name == "" || receipt.ProjectID == "" {
			return "", receipt.Name, receipt.ProjectID, errors.New("receipt has no exact workbook name and project")
		}
		matches, err := adapter.FindWorkbooks(ctx, receipt.Name, receipt.ProjectID)
		if err != nil {
			return "", receipt.Name, receipt.ProjectID, err
		}
		if len(matches) == 0 {
			return "", receipt.Name, receipt.ProjectID, nil
		}
		if len(matches) != 1 || matches[0].Name != receipt.Name || matches[0].ProjectLUID != receipt.ProjectID {
			return "", receipt.Name, receipt.ProjectID, fmt.Errorf("exact workbook destination is ambiguous or unavailable")
		}
		return matches[0].LUID, matches[0].Name, matches[0].ProjectLUID, nil
	case "datasource.publish":
		adapter := resourcedatasource.NewAdapterWithProjectResolver(clients.datasources, paths)
		if receipt.Observation.ResourceID != "" {
			item, err := adapter.ResolveDatasource(ctx, identity.Selector{LUID: identity.LUID(receipt.Observation.ResourceID)})
			if err != nil {
				return receipt.Observation.ResourceID, receipt.Name, receipt.ProjectID, err
			}
			return item.LUID, item.Name, item.ProjectLUID, nil
		}
		if receipt.Name == "" || receipt.ProjectID == "" {
			return "", receipt.Name, receipt.ProjectID, errors.New("receipt has no exact datasource name and project")
		}
		matches, err := adapter.FindDatasources(ctx, receipt.Name, receipt.ProjectID)
		if err != nil {
			return "", receipt.Name, receipt.ProjectID, err
		}
		if len(matches) == 0 {
			return "", receipt.Name, receipt.ProjectID, nil
		}
		if len(matches) != 1 || matches[0].Name != receipt.Name || matches[0].ProjectLUID != receipt.ProjectID {
			return "", receipt.Name, receipt.ProjectID, fmt.Errorf("exact datasource destination is ambiguous or unavailable")
		}
		return matches[0].LUID, matches[0].Name, matches[0].ProjectLUID, nil
	default:
		if receipt.Observation.ResourceID != "" {
			return receipt.Observation.ResourceID, receipt.Name, receipt.ProjectID, nil
		}
		return "", receipt.Name, receipt.ProjectID, fmt.Errorf("destination resolution is unsupported for operation %q", receipt.Operation)
	}
}

func (r *runtimeDependencies) inspectReceiptOnce(ctx context.Context, receipt jobmonitor.Receipt, connections map[string]authenticatedTableau) (jobmonitor.Receipt, error) {
	if receipt.Environment == "" || receipt.Observation.ID == "" {
		return receipt, errors.New("saved receipt has incomplete exact target or job identity")
	}
	if receipt.ConfigPath != "" && r.configPath != "" {
		expected, expectedErr := filepath.Abs(receipt.ConfigPath)
		actual, actualErr := filepath.Abs(r.configPath)
		if expectedErr == nil && actualErr == nil && expected != actual {
			return receipt, errors.New("saved receipt config path does not match the selected status configuration")
		}
	}
	connection, err := r.receiptConnection(ctx, receipt, connections)
	if err != nil {
		return receipt, err
	}
	status, err := job.NewClient(connection.transport, connection.session, connection.environment.URL).Inspect(ctx, receipt.Observation.ID)
	if err != nil {
		return receipt, err
	}
	if receipt.Observation.Type != "" && receipt.Observation.Type != status.Type {
		return receipt, errors.New("saved receipt job type changed during exact inspection")
	}
	receipt.Observation = status
	return receipt, nil
}

func (r *runtimeDependencies) receiptConnection(ctx context.Context, receipt jobmonitor.Receipt, connections map[string]authenticatedTableau) (authenticatedTableau, error) {
	key := receipt.Environment + "\x00" + receipt.Site + "\x00" + receipt.Server
	connection, ok := connections[key]
	if ok {
		return connection, nil
	}
	connection, err := r.tableauConnection(ctx, receipt.Environment, false)
	if err != nil {
		return connection, err
	}
	serverMismatch := receipt.Server != "" && normalizeServer(connection.environment.URL) != normalizeServer(receipt.Server)
	// An empty content URL is the Tableau default site. It is a real target,
	// not a wildcard, so a receipt for that site must not be checked through a
	// named site. Legacy receipts may omit SiteID, but a present ID is exact.
	siteMismatch := connection.environment.SiteContentURL != receipt.Site
	if receipt.Site == "" && connection.environment.SiteContentURL == "" {
		siteMismatch = false
	}
	siteIDMismatch := receipt.SiteID != "" && connection.session.SiteLUID() != receipt.SiteID
	if serverMismatch || siteMismatch || siteIDMismatch {
		return connection, errors.New("saved receipt target does not match the authenticated environment and site")
	}
	connections[key] = connection
	return connection, nil
}

func (r *runtimeDependencies) reconcilePublicationReceipts(ctx context.Context, record operationrun.Record, items []jobinspect.OperationItem) (*operationrun.Record, []string) {
	if len(items) == 0 || len(items) != len(record.ReceiptPaths) {
		return nil, nil
	}
	failed := record.Phase == operationrun.PhaseFailed || publicationHasFailure(record.FullResult) || publicationHasFailure(record.CompactResult)
	mergedFull := mergePublicationResult(record.FullResult, items, "succeeded", record.Operation)
	pending := publicationHasPending(mergedFull)
	for _, item := range items {
		switch item.Status {
		case "failed", "cancelled":
			failed = true
		case "succeeded":
		case "destination_pending", "pending", "running", "unknown", "":
			pending = true
		default:
			pending = true
		}
	}
	if pending {
		status := "pending"
		if failed {
			status = "partial_failure"
		}
		updated, err := publicationOperationStore(r.operationDirectory)
		if err != nil {
			return nil, []string{fmt.Sprintf("operation state could not be reopened for reconciliation: %v", err)}
		}
		result, err := updated.Update(record.ID, func(current *operationrun.Record) error {
			current.CompactResult = mergePublicationResult(current.CompactResult, items, status, current.Operation)
			current.FullResult = mergePublicationResult(current.FullResult, items, status, current.Operation)
			return nil
		})
		if err != nil {
			return nil, []string{fmt.Sprintf("partial receipt states could not be saved to operation %q: %v", record.ID, err)}
		}
		return &result, nil
	}
	nextPhase := operationrun.PhaseCompleted
	nextStatus := "succeeded"
	if failed {
		nextPhase = operationrun.PhaseFailed
		nextStatus = "failed"
	}
	updated, err := publicationOperationStore(r.operationDirectory)
	if err != nil {
		return nil, []string{fmt.Sprintf("operation state could not be reopened for reconciliation: %v", err)}
	}
	result, err := updated.Update(record.ID, func(current *operationrun.Record) error {
		current.Phase = nextPhase
		if current.FinishedAt.IsZero() {
			finishedAt := time.Now().UTC()
			if r.now != nil {
				finishedAt = r.now().UTC()
			}
			current.FinishedAt = finishedAt
		}
		current.CompactResult = mergePublicationResult(current.CompactResult, items, nextStatus, current.Operation)
		current.FullResult = mergePublicationResult(current.FullResult, items, nextStatus, current.Operation)
		if current.ExitCode == nil {
			code := 0
			if failed {
				code = 1
			}
			current.ExitCode = &code
		}
		return nil
	})
	if err != nil {
		return nil, []string{fmt.Sprintf("confirmed receipt states could not be saved to operation %q: %v", record.ID, err)}
	}
	return &result, nil
}

func mergePublicationResult(data []byte, items []jobinspect.OperationItem, status, operation string) []byte {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	value = mergePublicationValue(value, items, operation)
	if object, ok := value.(map[string]any); ok {
		object["status"] = status
		countPublicationItems(object)
	}
	merged, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return merged
}

func mergePublicationValue(value any, items []jobinspect.OperationItem, operation string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			typed[key] = mergePublicationValue(child, items, operation)
		}
		if item, ok := matchingPublicationItem(typed, items); ok {
			if item.Status != "unknown" && item.Status != "destination_pending" && item.Status != "" {
				typed["status"] = item.Status
			}
			setPublicationIdentity(typed, item, operation)
			if item.Verification != "" {
				typed["verification"] = item.Verification
			}
		}
		promotePublicationStatus(typed)
	case []any:
		for index, child := range typed {
			typed[index] = mergePublicationValue(child, items, operation)
		}
	}
	return value
}

func matchingPublicationItem(object map[string]any, items []jobinspect.OperationItem) (jobinspect.OperationItem, bool) {
	path, _ := object["receipt_path"].(string)
	jobID := ""
	for _, key := range []string{"tableau_job_id", "job_id"} {
		if value, ok := object[key].(string); ok {
			jobID = value
			break
		}
	}
	if path == "" && jobID == "" {
		return jobinspect.OperationItem{}, false
	}
	for _, item := range items {
		if path != "" && filepath.ToSlash(path) == filepath.ToSlash(item.ReceiptPath) {
			return item, true
		}
		if jobID != "" && item.JobID != "" && jobID == item.JobID {
			return item, true
		}
	}
	return jobinspect.OperationItem{}, false
}

func setPublicationIdentity(object map[string]any, item jobinspect.OperationItem, operation string) {
	if item.ResourceID != "" {
		switch operation {
		case "workbook.publish":
			object["workbook_luid"] = item.ResourceID
			delete(object, "resource_id")
		case "datasource.publish":
			object["datasource_luid"] = item.ResourceID
			delete(object, "resource_id")
		case "flow.publish":
			object["flow_luid"] = item.ResourceID
			delete(object, "resource_id")
		default:
			if _, exists := object["resource_id"]; exists {
				object["resource_id"] = item.ResourceID
			}
		}
	}
	if item.Name != "" {
		switch operation {
		case "workbook.publish":
			object["workbook_name"] = item.Name
		case "datasource.publish":
			object["datasource_name"] = item.Name
		case "flow.publish":
			object["flow_name"] = item.Name
		default:
			if _, exists := object["name"]; exists {
				object["name"] = item.Name
			}
		}
	}
	if item.Project != "" {
		if _, exists := object["project_luid"]; exists {
			object["project_luid"] = item.Project
		} else if _, exists := object["project_id"]; exists {
			object["project_id"] = item.Project
		} else if operation == "workbook.publish" || operation == "datasource.publish" || operation == "flow.publish" {
			object["project_luid"] = item.Project
		}
	}
}

func promotePublicationStatus(object map[string]any) {
	current, _ := object["status"].(string)
	if current != "" && current != "pending" && current != "running" && current != "queued" && current != "accepted" && current != "unknown" {
		return
	}
	nested, ok := object["result"].(map[string]any)
	if !ok {
		return
	}
	status, _ := nested["status"].(string)
	if status == "succeeded" || status == "failed" || status == "cancelled" || status == "skipped" || status == "destination_pending" {
		object["status"] = status
	}
}

func countPublicationItems(object map[string]any) {
	items, ok := object["items"].([]any)
	if !ok {
		return
	}
	previousStatus, _ := object["status"].(string)
	succeeded, failed, skipped, pending := 0, 0, 0, 0
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			pending++
			continue
		}
		switch entry["status"] {
		case "succeeded":
			succeeded++
		case "failed":
			failed++
		case "skipped":
			skipped++
		default:
			pending++
		}
	}
	object["succeeded"], object["failed"], object["skipped"] = succeeded, failed, skipped
	if pending > 0 {
		object["pending"] = pending
	} else {
		delete(object, "pending")
	}
	if failed+skipped > 0 {
		object["status"] = "partial_failure"
		if succeeded == 0 && pending == 0 {
			object["status"] = "failed"
		}
	} else if previousStatus == "partial_failure" || previousStatus == "failed" {
		object["status"] = previousStatus
	} else if pending > 0 {
		if previousStatus == "pending" {
			object["status"] = "pending"
		} else {
			object["status"] = "running"
		}
	} else {
		object["status"] = "succeeded"
	}
}

func publicationHasFailure(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(value any) bool {
		switch typed := value.(type) {
		case map[string]any:
			if status, _ := typed["status"].(string); status == "failed" || status == "partial_failure" || status == "skipped" {
				return true
			}
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func publicationHasPending(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(value any) bool {
		switch typed := value.(type) {
		case map[string]any:
			if status, _ := typed["status"].(string); status == "pending" || status == "running" || status == "queued" || status == "unknown" || status == "destination_pending" {
				return true
			}
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func publicationSnapshotStatus(record operationrun.Record) string {
	for _, data := range [][]byte{record.FullResult, record.CompactResult, record.LiveResults} {
		if len(data) == 0 {
			continue
		}
		var value map[string]any
		if json.Unmarshal(data, &value) == nil {
			if status, ok := value["status"].(string); ok {
				return status
			}
		}
	}
	return ""
}

func publicationNeedsDestination(data []byte) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(value any) bool {
		switch object := value.(type) {
		case map[string]any:
			if verification, _ := object["verification"].(string); strings.HasPrefix(verification, "destination_") {
				return true
			}
			for _, child := range object {
				if visit(child) {
					return true
				}
			}
		case []any:
			for _, child := range object {
				if visit(child) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}

func publicationTarget(record operationrun.Record) (string, string) {
	for _, data := range [][]byte{record.FullResult, record.CompactResult, record.LiveResults} {
		if environment, site, ok := findPublicationTarget(data); ok {
			return environment, site
		}
	}
	return "", ""
}

func findPublicationTarget(data []byte) (string, string, bool) {
	if len(data) == 0 {
		return "", "", false
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return "", "", false
	}
	var visit func(any) (string, string, bool)
	visit = func(value any) (string, string, bool) {
		switch typed := value.(type) {
		case map[string]any:
			environment, _ := typed["environment"].(string)
			site, _ := typed["site"].(string)
			if environment != "" || site != "" {
				return environment, site, true
			}
			for _, child := range typed {
				if environment, site, ok := visit(child); ok {
					return environment, site, true
				}
			}
		case []any:
			for _, child := range typed {
				if environment, site, ok := visit(child); ok {
					return environment, site, true
				}
			}
		}
		return "", "", false
	}
	return visit(value)
}

func (r *runtimeDependencies) publicationReceiptStore() (jobmonitor.Store, error) {
	directory := r.jobDirectory
	if directory == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return jobmonitor.Store{}, err
		}
		directory = filepath.Join(cache, "tadx", "jobs")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return jobmonitor.Store{}, err
	}
	return jobmonitor.Store{Directory: absolute}, nil
}

func receiptKey(index int, path string) string {
	if strings.TrimSpace(path) != "" {
		return filepath.ToSlash(path)
	}
	return fmt.Sprintf("receipt-%d", index+1)
}
