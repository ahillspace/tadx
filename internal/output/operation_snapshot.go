package output

import (
	"encoding/json"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	shared "github.com/ahillspace/tadx/internal/value"
)

func CaptureOperationSnapshots(value any, configPath string, maxBytes int) (json.RawMessage, json.RawMessage, error) {
	full, err := SnapshotWithConfig(value, maxBytes, configPath)
	if err != nil {
		return nil, nil, err
	}
	if projector, ok := value.(CompactProjector); ok {
		value = projector.CompactOutput()
	}
	if carried, ok := value.(error); ok {
		if partial, ok := errors.AsType[interface {
			error
			OperationOutput() any
		}](carried); ok {
			v := partial.OperationOutput()
			if p, ok := v.(CompactProjector); ok {
				v = p.CompactOutput()
			}
			value = map[string]any{"output": v, "error": errs.Structure(carried).Error}
		}
	}
	compact, err := SnapshotWithConfig(value, maxBytes, configPath)
	return compact, full, err
}

func OperationContainsUnfinished(data json.RawMessage) bool {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var pending func(any) bool
	pending = func(value any) bool {
		switch v := value.(type) {
		case map[string]any:
			if state, _ := v["status"].(string); state == "pending" || state == "running" || state == "queued" || state == "accepted" {
				return true
			}
			for _, child := range v {
				if pending(child) {
					return true
				}
			}
		case []any:
			for _, child := range v {
				if pending(child) {
					return true
				}
			}
		}
		return false
	}
	return pending(value)
}

func OperationCheckCommand(record shared.OperationRecord) string {
	args := []string{"job", "inspect", "--operation-id", record.ID}
	if record.ConfigPath != "" {
		args = append(args, "--config", record.ConfigPath)
	}
	return commandhint.Command(args...)
}

type OperationRun struct {
	Record  shared.OperationRecord `json:"-"`
	Stopped bool                   `json:"-"`
}

func (o OperationRun) CompactOutput() any {
	return OperationSnapshot(o.Record, false, o.Stopped)
}
func (o OperationRun) FullOutput() any { return OperationSnapshot(o.Record, true, o.Stopped) }

func OperationSnapshot(record shared.OperationRecord, full, stopped bool) any {
	data := record.CompactResult
	if full {
		data = record.FullResult
	}
	if len(data) == 0 {
		data = record.LiveResults
	}
	result := map[string]any{}
	if len(data) > 0 {
		_ = json.Unmarshal(data, &result)
	}
	if result == nil {
		result = map[string]any{}
	}
	result["operation_id"], result["operation"] = record.ID, record.Operation
	if _, ok := result["status"]; !ok {
		state := "running"
		if record.Phase == "requested" {
			state = "starting"
		}
		if record.Phase == "completed" {
			state = "succeeded"
		}
		if record.Phase == "failed" {
			state = "failed"
		}
		result["status"] = state
	}
	if record.Phase != "completed" || OperationContainsUnfinished(record.FullResult) || OperationNeedsDestination(record.FullResult) {
		result["check_status"] = OperationCheckCommand(record)
	}
	if stopped {
		result["waiting_stopped"] = true
	}
	if !full {
		compactOperationSnapshot(result)
		if _, checking := result["check_status"]; !checking {
			result["details"] = OperationCheckCommand(record) + " --full"
		}
	}
	return result
}

// The operation handle replaces per-item recovery paths in compact
// Saved full results retain the original action contracts and receipt paths.
func compactOperationSnapshot(result map[string]any) {
	items, batch := result["items"].([]any)
	if batch {
		for _, field := range []string{"environment", "site", "workspace", "project_path", "kind"} {
			common := ""
			shared := len(items) > 0
			for index, raw := range items {
				item, _ := raw.(map[string]any)
				value, _ := item["result"].(map[string]any)
				candidate, _ := value[field].(string)
				if candidate == "" || (index > 0 && common != candidate) {
					shared = false
					break
				}
				common = candidate
			}
			if shared {
				result[field] = common
				for _, raw := range items {
					item := raw.(map[string]any)
					delete(item["result"].(map[string]any), field)
				}
			}
		}
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			value, _ := item["result"].(map[string]any)
			if value == nil {
				continue
			}
			if native, ok := value["result"].(map[string]any); ok {
				delete(value, "result")
				for key, v := range native {
					value[key] = v
				}
			}
			if value["status"] == item["status"] {
				delete(value, "status")
			}
			if value["operation"] == result["operation"] {
				delete(value, "operation")
			}
		}
	}
	var trim func(any)
	trim = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			delete(v, "receipt_path")
			delete(v, "details")
			if help, ok := v["help"].([]any); ok && len(help) == 0 {
				delete(v, "help")
			}
			for _, child := range v {
				trim(child)
			}
		case []any:
			for _, child := range v {
				trim(child)
			}
		}
	}
	trim(result)
}
