package operationrun

import "github.com/ahillspace/tadx/internal/value"

// OutputRecord exposes snapshot data without coupling storage to rendering.
func (r Record) OutputRecord() value.OperationRecord {
	return value.OperationRecord{ID: r.ID, Operation: r.Operation, Phase: string(r.Phase), ConfigPath: r.Request.ConfigPath, CompactResult: r.CompactResult, FullResult: r.FullResult, LiveResults: r.LiveResults}
}
