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
	var err error
	input, err = NormalizeInput(input)
	if err != nil {
		return Output{}, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if input.All {
		limit = maxAllFields
	}
	fingerprint := inputFingerprint(input)
	offset, err := decodeCursor(input.Cursor, fingerprint)
	if err != nil {
		return Output{}, schemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid for this query.", err, "Restart without --cursor; increase --limit or use --all to inspect more fields.")
	}
	value, err := a.reader.ReadDatasourceSchema(ctx, input.DatasourceLUID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Verify datasource access and Metadata API availability, then retry.")
		return Output{}, &errs.Error{ID: "datasource.schema.read", Kind: errs.KindOperation, Operation: "datasource.schema", Environment: input.Environment, Site: input.Site, Summary: "Datasource schema discovery failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if strings.TrimSpace(value.DatasourceLUID) != input.DatasourceLUID || strings.TrimSpace(value.DatasourceName) == "" {
		return Output{}, schemaError("datasource.schema.identity_mismatch", errs.KindOperation, input, "Datasource schema returned an incomplete or mismatched identity.", nil, "Retry after Tableau returns an authoritative datasource identity.")
	}

	if (input.Descriptions && !value.DescriptionsObserved) || (input.Tags && !value.TagsObserved) {
		return Output{}, schemaError("datasource.schema.metadata_not_indexed", errs.KindOperation, input, "Requested field metadata has not been observed in this schema.", nil, "Run schema with the requested --descriptions or --tags flags without --cache.")
	}
	fields, err := filterFields(value.Fields, input)
	if err != nil {
		return Output{}, err
	}
	if input.All && len(fields) > maxAllFields {
		return Output{}, schemaError("datasource.schema.incomplete", errs.KindOperation, input, "The matching schema exceeds the 10,000-field inventory bound.", nil, "Narrow the schema with --role, --table, or --query before using --all.")
	}
	if offset < 0 || offset > len(fields) {
		return Output{}, schemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor exceeds the current result set.", nil, "Restart without --cursor.")
	}
	end := min(offset+limit, len(fields))
	fields = projectMetadata(fields, input)
	next := ""
	if end < len(fields) {
		next = encodeCursor(end, fingerprint)
	}
	observedAt := strings.TrimSpace(value.ObservedAt)
	if observedAt == "" {
		observedAt = a.now().UTC().Format(time.RFC3339Nano)
	}
	source := readsource.Live(parseObservedAt(observedAt, a.now()))
	if input.Cache {
		// The composition root replaces this with authoritative cache generation
		// metadata. This action has no generation provenance of its own, so the
		// placeholder must not claim complete, fresh coverage: if it is ever
		// surfaced unwrapped it stays honest as partial and stale.
		source = readsource.Cached(parseObservedAt(observedAt, a.now()), readsource.CoveragePartial, "", time.Time{}, true)
	}
	return Output{
		Status: "listed", Environment: input.Environment, Site: input.Site,
		DatasourceLUID: value.DatasourceLUID, DatasourceName: value.DatasourceName,
		Tables: append([]Table(nil), value.Tables...), Page: Page{Returned: end - offset, Total: len(fields), Limit: limit, NextCursor: next, MoreAvailable: next != ""},
		Fields: append([]Field(nil), fields[offset:end]...), Warnings: append([]string(nil), value.Warnings...),
		Source: &source, RequestID: value.RequestID,
	}, nil
}

func normalizeFieldSelection(input Input) (Input, error) {
	selected := make(map[string]struct{}, len(input.FieldIDs)+1)
	ids := input.FieldIDs
	if input.FieldID != "" {
		ids = append(append([]string(nil), ids...), input.FieldID)
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return input, schemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field identifiers cannot be empty.", nil, "Provide an exact raw Tableau field identifier for each --field-id.")
		}
		selected[id] = struct{}{}
		if len(selected) > maxAllFields {
			return input, schemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field selection exceeds the 10,000-field inventory bound.", nil, "Select at most 10,000 distinct field identifiers.")
		}
	}
	input.FieldIDs = make([]string, 0, len(selected))
	for id := range selected {
		input.FieldIDs = append(input.FieldIDs, id)
	}
	sort.Strings(input.FieldIDs)
	input.FieldID = ""
	if len(input.FieldIDs) == 1 {
		input.FieldID = input.FieldIDs[0]
	}
	return input, nil
}

func filterFields(fields []Field, input Input) ([]Field, error) {
	query := strings.ToLower(input.Query)
	table := strings.ToLower(input.Table)
	selected := make(map[string]int, len(input.FieldIDs))
	for _, id := range input.FieldIDs {
		selected[id] = 0
	}
	result := make([]Field, 0, len(fields))
	for _, field := range fields {
		role := strings.ToLower(strings.TrimSpace(field.Role))
		if input.Role != "" && role != input.Role {
			continue
		}
		if len(selected) > 0 {
			if _, requested := selected[field.ID]; !requested {
				continue
			}
		}
		if table != "" && strings.ToLower(field.Table) != table {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{field.ID, field.Name, field.Caption, field.Label, field.Formula}, "\x00"))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		if len(selected) > 0 {
			selected[field.ID]++
		}
		result = append(result, field)
	}
	for _, id := range input.FieldIDs {
		switch matches := selected[id]; {
		case matches == 0:
			return nil, schemaError("datasource.schema.field_not_found", errs.KindUsage, input, fmt.Sprintf("Datasource field identifier %q did not match the selected schema.", id), nil, "Verify the exact --field-id and remove any conflicting --role, --table, or --query filters.")
		case matches > 1:
			return nil, schemaError("datasource.schema.field_ambiguous", errs.KindUsage, input, fmt.Sprintf("Datasource field identifier %q matched %d fields.", id, matches), nil, "Inspect the field inventory and use --table to select the intended logical table.")
		}
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
	return result, nil
}

type cursor struct {
	Offset      int    `json:"offset"`
	Fingerprint string `json:"fingerprint"`
}

func inputFingerprint(input Input) string {
	value := strings.Join([]string{input.Environment, input.Site, input.DatasourceLUID, input.Query, input.Role, input.Table, input.FieldID, fmt.Sprintf("%t", input.Cache)}, "\x00")
	if len(input.FieldIDs) > 1 {
		// Preserve legacy fingerprints for zero or one selector, while binding
		// multi-selection cursors to the complete exact, order-independent set.
		selection, _ := json.Marshal(input.FieldIDs)
		value += "\x00" + string(selection)
	}
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
