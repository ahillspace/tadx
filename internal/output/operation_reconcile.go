package output

import (
	"encoding/json"
	"path/filepath"
	"strings"

	shared "github.com/ahillspace/tadx/internal/value"
)

func MergeOperationResult(data []byte, items []shared.OperationItem, status, operation string) []byte {
	if len(data) == 0 {
		return data
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return data
	}
	value = mergeOperationValue(value, items, operation)
	if object, ok := value.(map[string]any); ok {
		object["status"] = status
		countOperationItems(object)
	}
	merged, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return merged
}

func mergeOperationValue(value any, items []shared.OperationItem, operation string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			typed[key] = mergeOperationValue(child, items, operation)
		}
		if item, ok := matchingOperationItem(typed, items); ok {
			if item.Status != "unknown" && item.Status != "destination_pending" && item.Status != "" {
				typed["status"] = item.Status
			}
			setOperationIdentity(typed, item, operation)
			if item.Verification != "" {
				typed["verification"] = item.Verification
			}
		}
		promoteOperationStatus(typed)
	case []any:
		for index, child := range typed {
			typed[index] = mergeOperationValue(child, items, operation)
		}
	}
	return value
}

func matchingOperationItem(object map[string]any, items []shared.OperationItem) (shared.OperationItem, bool) {
	path, _ := object["receipt_path"].(string)
	jobID := ""
	for _, key := range []string{"tableau_job_id", "job_id"} {
		if value, ok := object[key].(string); ok {
			jobID = value
			break
		}
	}
	if path == "" && jobID == "" {
		return shared.OperationItem{}, false
	}
	for _, item := range items {
		if path != "" && filepath.ToSlash(path) == filepath.ToSlash(item.ReceiptPath) {
			return item, true
		}
		if jobID != "" && item.JobID != "" && jobID == item.JobID {
			return item, true
		}
	}
	return shared.OperationItem{}, false
}

func setOperationIdentity(object map[string]any, item shared.OperationItem, operation string) {
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

func promoteOperationStatus(object map[string]any) {
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

func countOperationItems(object map[string]any) {
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

func OperationHasFailure(data []byte) bool {
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

func OperationHasPending(data []byte) bool {
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

func OperationSnapshotStatus(record shared.OperationRecord) string {
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

func OperationNeedsDestination(data []byte) bool {
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

func OperationTarget(record shared.OperationRecord) (string, string) {
	for _, data := range [][]byte{record.FullResult, record.CompactResult, record.LiveResults} {
		if environment, site, ok := findOperationTarget(data); ok {
			return environment, site
		}
	}
	return "", ""
}

func findOperationTarget(data []byte) (string, string, bool) {
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
