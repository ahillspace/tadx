// Package schema discovers one published datasource's logical tables and fields.
package schema

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// Reader is the action-owned datasource schema seam.
type Reader interface {
	ReadDatasourceSchema(context.Context, string) (Schema, error)
}

// Action returns a bounded, deterministic field view.
type Action struct {
	reader Reader
	now    func() time.Time
}

// New creates a datasource schema action.
func New(reader Reader, now func() time.Time) *Action {
	if now == nil {
		now = time.Now
	}
	return &Action{reader: reader, now: now}
}

// Execute reads, filters, and bounds one datasource schema.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, schemaError("datasource.schema.unconfigured", errs.KindRuntime, input, "Datasource schema discovery is not configured.", nil, "Configure the datasource schema reader before retrying.")
	}
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.DatasourceLUID = strings.TrimSpace(input.DatasourceLUID)
	input.Query = strings.TrimSpace(input.Query)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.Table = strings.TrimSpace(input.Table)
	input.FieldID = strings.TrimSpace(input.FieldID)
	if input.DatasourceLUID == "" {
		return Output{}, schemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource schema discovery requires --id.", nil, "Provide one authoritative datasource LUID with --id.")
	}
	if input.Role != "" && input.Role != "measure" && input.Role != "dimension" && input.Role != "date" && input.Role != "excluded" {
		return Output{}, schemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field role is invalid.", nil, "Use measure, dimension, date, or excluded.")
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return Output{}, schemaError("datasource.schema.usage", errs.KindUsage, input, fmt.Sprintf("Datasource schema limit must be between 1 and %d.", maxLimit), nil, fmt.Sprintf("Set --limit between 1 and %d.", maxLimit))
	}

	fingerprint := inputFingerprint(input)
	offset, err := decodeCursor(input.Cursor, fingerprint)
	if err != nil {
		return Output{}, schemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid for this query.", err, "Restart without --cursor, then use the returned continuation cursor unchanged.")
	}
	value, err := a.reader.ReadDatasourceSchema(ctx, input.DatasourceLUID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Verify datasource access and Metadata API availability, then retry.")
		return Output{}, &errs.Error{ID: "datasource.schema.read", Kind: errs.KindOperation, Operation: "datasource.schema", Environment: input.Environment, Site: input.Site, Summary: "Datasource schema discovery failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if strings.TrimSpace(value.DatasourceLUID) != input.DatasourceLUID || strings.TrimSpace(value.DatasourceName) == "" {
		return Output{}, schemaError("datasource.schema.identity_mismatch", errs.KindOperation, input, "Datasource schema returned an incomplete or mismatched identity.", nil, "Retry after Tableau returns an authoritative datasource identity.")
	}

	fields := filterFields(value.Fields, input)
	if offset < 0 || offset > len(fields) {
		return Output{}, schemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor exceeds the current result set.", nil, "Restart without --cursor.")
	}
	end := min(offset+limit, len(fields))
	next := ""
	if end < len(fields) {
		next = encodeCursor(end, fingerprint)
	}
	observedAt := strings.TrimSpace(value.ObservedAt)
	if observedAt == "" {
		observedAt = a.now().UTC().Format(time.RFC3339Nano)
	}
	source := readsource.Live(parseObservedAt(observedAt, a.now()))
	if input.Catalog {
		// The composition root replaces this with authoritative catalog generation metadata.
		source = readsource.Cached(parseObservedAt(observedAt, a.now()), readsource.CoverageComplete, "", time.Time{}, false)
	}
	return Output{
		Status: "listed", Environment: input.Environment, Site: input.Site,
		DatasourceLUID: value.DatasourceLUID, DatasourceName: value.DatasourceName,
		Tables: append([]Table(nil), value.Tables...), Page: Page{Returned: end - offset, Total: len(fields), Limit: limit, NextCursor: next},
		Fields: append([]Field(nil), fields[offset:end]...), Warnings: append([]string(nil), value.Warnings...),
		Source: &source, RequestID: value.RequestID,
		Help: []string{"tadx pulse definition create -h"},
	}, nil
}

func filterFields(fields []Field, input Input) []Field {
	query := strings.ToLower(input.Query)
	table := strings.ToLower(input.Table)
	result := make([]Field, 0, len(fields))
	for _, field := range fields {
		role := strings.ToLower(strings.TrimSpace(field.Role))
		if input.Role != "" && role != input.Role {
			continue
		}
		if input.FieldID != "" && field.ID != input.FieldID {
			continue
		}
		if table != "" && strings.ToLower(field.Table) != table {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{field.ID, field.Name, field.Caption, field.Label, field.Table, field.Formula}, "\x00"))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		result = append(result, field)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if left.Table != right.Table {
			return left.Table < right.Table
		}
		if left.Caption != right.Caption {
			return left.Caption < right.Caption
		}
		return left.ID < right.ID
	})
	return result
}

type cursor struct {
	Offset      int    `json:"offset"`
	Fingerprint string `json:"fingerprint"`
}

func inputFingerprint(input Input) string {
	value := strings.Join([]string{input.Environment, input.Site, input.DatasourceLUID, input.Query, input.Role, input.Table, input.FieldID, fmt.Sprintf("%t", input.Catalog)}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}

func encodeCursor(offset int, fingerprint string) string {
	value, _ := json.Marshal(cursor{Offset: offset, Fingerprint: fingerprint})
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeCursor(value, fingerprint string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var decoded cursor
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return 0, err
	}
	if decoded.Offset < 0 || decoded.Fingerprint != fingerprint {
		return 0, fmt.Errorf("cursor does not match this datasource schema query")
	}
	return decoded.Offset, nil
}

func parseObservedAt(value string, fallback time.Time) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fallback.UTC()
	}
	return parsed.UTC()
}

func schemaError(id string, kind errs.Kind, input Input, summary string, cause error, corrective string) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "datasource.schema", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: corrective}
}
