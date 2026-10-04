package job

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func (r recovery) recoverReceiptLinks(ctx context.Context, store operationrun.Store, record operationrun.Record) (operationrun.Record, []string) {
	receiptStore, err := r.ports.ReceiptStore()
	if err != nil {
		return record, []string{err.Error()}
	}
	linked := make(map[string]bool, len(record.ReceiptPaths))
	pathsByID := make(map[string]string, len(record.ReceiptPaths))
	for _, path := range record.ReceiptPaths {
		rawPath := path
		if !filepath.IsAbs(path) {
			path = filepath.Join(receiptStore.Directory, path)
		}
		if receipt, readErr := receiptStore.ReadPath(path); readErr == nil {
			linked[receipt.ReceiptID] = true
			pathsByID[receipt.ReceiptID] = rawPath
		}
	}
	intents := make([]operationrun.ReceiptIntent, 0, len(record.ReceiptIntents))
	ids := make([]string, 0, len(record.ReceiptIntents))
	for _, intent := range record.ReceiptIntents {
		if !linked[intent.ID] {
			intents = append(intents, intent)
			ids = append(ids, intent.ID)
		}
	}
	if len(ids) == 0 {
		return record, nil
	}
	found, err := receiptStore.FindByReceiptIDs(ctx, ids)
	if err != nil {
		return record, []string{fmt.Sprintf("registered publication receipts could not be located: %v", err)}
	}
	var paths, warnings []string
	for _, intent := range intents {
		located, ok := found[intent.ID]
		if !ok {
			continue
		}
		receipt := located.Receipt
		recoverableReceipt := receipt.Observation.ID != "" || (receipt.Observation.Status == "succeeded" && receipt.Observation.ResourceID != "") || receipt.Observation.Status == "unknown"
		if receipt.OperationID != record.ID || receipt.Operation != record.Operation || jobmonitor.ReceiptScope(receipt) != intent.Scope || receipt.AcceptedAt.Before(intent.RegisteredAt) || !recoverableReceipt || filepath.Clean(located.Path) != filepath.Clean(receiptStore.Path(receipt)) {
			warnings = append(warnings, fmt.Sprintf("registered receipt %q does not match its saved operation intent", intent.ID))
			continue
		}
		paths = append(paths, located.Path)
		pathsByID[intent.ID] = located.Path
	}
	if len(paths) == 0 {
		return record, warnings
	}
	updated, err := store.Update(record.ID, func(current *operationrun.Record) error {
		for _, path := range paths {
			if !slices.Contains(current.ReceiptPaths, path) {
				current.ReceiptPaths = append(current.ReceiptPaths, path)
			}
		}
		// Intents were registered before each write in batch order. Preserve
		// that exact order among known receipts and retain other saved paths.
		ordered := make([]string, 0, len(current.ReceiptPaths))
		present := make(map[string]bool, len(current.ReceiptPaths))
		for _, path := range current.ReceiptPaths {
			present[path] = true
		}
		used := make(map[string]bool, len(current.ReceiptPaths))
		for _, intent := range current.ReceiptIntents {
			if path, ok := pathsByID[intent.ID]; ok && present[path] && !used[path] {
				ordered = append(ordered, path)
				used[path] = true
			}
		}
		for _, path := range current.ReceiptPaths {
			if !used[path] {
				ordered = append(ordered, path)
			}
		}
		current.ReceiptPaths = ordered
		return nil
	})
	if err != nil {
		return record, append(warnings, fmt.Sprintf("registered publication receipts could not be linked: %v", err))
	}
	return updated, warnings
}

func (r recovery) readSavedReceipts(ctx context.Context, record operationrun.Record) ([]string, []OperationItem) {
	receiptStore, err := r.ports.ReceiptStore()
	if err != nil {
		return []string{err.Error()}, nil
	}
	warnings := make([]string, 0)
	items := make([]OperationItem, 0, min(len(record.ReceiptPaths), maxOperationReceiptChecks))
	connections := make(map[string]RecoverySession)
	for index, rawPath := range record.ReceiptPaths {
		if index >= maxOperationReceiptChecks {
			warnings = append(warnings, fmt.Sprintf("receipt display limited to %d of %d saved paths", maxOperationReceiptChecks, len(record.ReceiptPaths)))
			break
		}
		path := rawPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(receiptStore.Directory, path)
		}
		item := OperationItem{Key: receiptKey(index, path), ReceiptPath: filepath.ToSlash(path)}
		receipt, readErr := receiptStore.ReadPath(path)
		if readErr != nil {
			item.Status, item.Error = "unknown", readErr.Error()
			warnings = append(warnings, fmt.Sprintf("receipt %q could not be read: %v", filepath.ToSlash(path), readErr))
		} else {
			item.Name, item.Project = receipt.Name, receipt.ProjectID
			item.Status, item.ResourceID = receipt.Observation.Status, receipt.Observation.ResourceID
			item.JobID = receipt.Observation.ID
			item.TableauRequestID = receipt.Observation.RequestID
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

func (r recovery) savedTarget(record operationrun.Record) (string, string) {
	receiptStore, err := r.ports.ReceiptStore()
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

func (r recovery) inspectReceipts(ctx context.Context, record operationrun.Record) ([]string, []OperationItem, string, string) {
	receiptStore, err := r.ports.ReceiptStore()
	if err != nil {
		return []string{err.Error()}, nil, "", ""
	}
	warnings := make([]string, 0)
	items := make([]OperationItem, 0, min(len(record.ReceiptPaths), maxOperationReceiptChecks))
	environment, site := "", ""
	if len(record.ReceiptPaths) > maxOperationReceiptChecks {
		warnings = append(warnings, fmt.Sprintf("receipt checks limited to %d of %d saved paths", maxOperationReceiptChecks, len(record.ReceiptPaths)))
	}
	connections := make(map[string]RecoverySession)
	for index, rawPath := range record.ReceiptPaths {
		if index >= maxOperationReceiptChecks {
			break
		}
		path := rawPath
		if !filepath.IsAbs(path) {
			path = filepath.Join(receiptStore.Directory, path)
		}
		receipt, readErr := receiptStore.ReadPath(path)
		item := OperationItem{Key: receiptKey(index, path), ReceiptPath: filepath.ToSlash(path)}
		if readErr != nil {
			item.Status = "unknown"
			item.Error = readErr.Error()
			warnings = append(warnings, fmt.Sprintf("receipt %q could not be read: %v", filepath.ToSlash(path), readErr))
			items = append(items, item)
			continue
		}
		item.Name, item.Project = receipt.Name, receipt.ProjectID
		item.JobID = receipt.Observation.ID
		item.TableauRequestID = receipt.Observation.RequestID
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
		if receipt.Observation.ID == "" {
			// Synchronous publish outcomes have no Tableau job to inspect.
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

func (r recovery) resolveReceiptItem(ctx context.Context, item OperationItem, receipt jobmonitor.Receipt, connections map[string]RecoverySession, warnings []string) (OperationItem, jobmonitor.Receipt, []string) {
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
	destination, err := connection.ResolveDestination(ctx, value.PublicationDestination{Operation: receipt.Operation, ResourceID: receipt.Observation.ResourceID, Name: receipt.Name, ProjectID: receipt.ProjectID})
	resourceID, name, project := destination.ResourceID, destination.Name, destination.ProjectID
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

func (r recovery) inspectReceiptOnce(ctx context.Context, receipt jobmonitor.Receipt, connections map[string]RecoverySession) (jobmonitor.Receipt, error) {
	if receipt.Environment == "" || receipt.Observation.ID == "" {
		return receipt, errors.New("saved receipt has incomplete exact target or job identity")
	}
	if receipt.ConfigPath != "" && r.ports.ConfigPath != "" {
		expected, expectedErr := filepath.Abs(receipt.ConfigPath)
		actual, actualErr := filepath.Abs(r.ports.ConfigPath)
		if expectedErr == nil && actualErr == nil && expected != actual {
			return receipt, errors.New("saved receipt config path does not match the selected status configuration")
		}
	}
	connection, err := r.receiptConnection(ctx, receipt, connections)
	if err != nil {
		return receipt, err
	}
	status, err := connection.Inspect(ctx, receipt.Observation.ID)
	if err != nil {
		return receipt, err
	}
	if receipt.Observation.Type != "" && receipt.Observation.Type != status.Type {
		return receipt, errors.New("saved receipt job type changed during exact inspection")
	}
	receipt.Observation = status
	return receipt, nil
}

func (r recovery) receiptConnection(ctx context.Context, receipt jobmonitor.Receipt, connections map[string]RecoverySession) (RecoverySession, error) {
	key := receipt.Environment + "\x00" + receipt.Site + "\x00" + receipt.Server
	connection, ok := connections[key]
	if ok {
		return connection, nil
	}
	connection, err := r.ports.Open(ctx, receipt.Environment)
	if err != nil {
		return connection, err
	}
	serverMismatch := receipt.Server != "" && jobmonitor.NormalizeServer(connection.Server) != jobmonitor.NormalizeServer(receipt.Server)
	// An empty content URL is the Tableau default site. It is a real target,
	// not a wildcard, so a receipt for that site must not be checked through a
	// named site. Legacy receipts may omit SiteID, but a present ID is exact.
	siteMismatch := connection.Site != receipt.Site
	if receipt.Site == "" && connection.Site == "" {
		siteMismatch = false
	}
	siteIDMismatch := receipt.SiteID != "" && connection.SiteID != receipt.SiteID
	if serverMismatch || siteMismatch || siteIDMismatch {
		return connection, errors.New("saved receipt target does not match the authenticated environment and site")
	}
	connections[key] = connection
	return connection, nil
}

func receiptKey(index int, path string) string {
	if strings.TrimSpace(path) != "" {
		return filepath.ToSlash(path)
	}
	return fmt.Sprintf("receipt-%d", index+1)
}
