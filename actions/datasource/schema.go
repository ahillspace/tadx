package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
	"sort"
	"strings"
	"time"
)

// SchemaReader is the action-owned datasource schema seam.
type SchemaReader interface {
	ReadDatasourceSchema(context.Context, string) (SchemaRecord, error)
}

// Schema reads, filters, and bounds one datasource schema.
func Schema(ctx context.Context, reader SchemaReader, now func() time.Time, input SchemaInput) (SchemaOutput, error) {
	if reader == nil {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.unconfigured", errs.KindRuntime, input, "Datasource schema discovery is not configured.", nil, "Configure the datasource schema reader before retrying.")
	}
	if now == nil {
		now = time.Now
	}
	var err error
	input, err = SchemaNormalizeInput(input)
	if err != nil {
		return SchemaOutput{}, err
	}
	limit := input.Limit
	if limit == 0 {
		limit = schemaDefaultLimit
	}
	if input.All {
		limit = schemaMaxAllFields
	}
	fingerprint := schemaInputFingerprint(input)
	offset, err := schemaDecodeCursor(input.Cursor, fingerprint)
	if err != nil {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid for this query.", err, "Restart without --cursor; increase --limit or use --all to inspect more fields.")
	}
	value, err := reader.ReadDatasourceSchema(ctx, input.DatasourceLUID)
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Verify datasource access and Metadata API availability, then retry.")
		return SchemaOutput{}, &errs.Error{ID: "datasource.schema.read", Kind: errs.KindOperation, Operation: "datasource.schema", Environment: input.Environment, Site: input.Site, Summary: "Datasource schema discovery failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if strings.TrimSpace(value.DatasourceLUID) != input.DatasourceLUID || strings.TrimSpace(value.DatasourceName) == "" {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.identity_mismatch", errs.KindOperation, input, "Datasource schema returned an incomplete or mismatched identity.", nil, "Retry after Tableau returns an authoritative datasource identity.")
	}

	if (input.Descriptions && !value.DescriptionsObserved) || (input.Tags && !value.TagsObserved) {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.metadata_not_indexed", errs.KindOperation, input, "Requested field metadata has not been observed in this schema.", nil, "Run schema with the requested --descriptions or --tags flags without --cache.")
	}
	fields, err := schemaFilterFields(value.Fields, input)
	if err != nil {
		return SchemaOutput{}, err
	}
	if input.All && len(fields) > schemaMaxAllFields {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.incomplete", errs.KindOperation, input, "The matching schema exceeds the 10,000-field inventory bound.", nil, "Narrow the schema with --role, --table, or --query before using --all.")
	}
	if offset < 0 || offset > len(fields) {
		return SchemaOutput{}, schemaSchemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor exceeds the current result set.", nil, "Restart without --cursor.")
	}
	end := min(offset+limit, len(fields))
	fields = schemaProjectMetadata(fields, input)
	next := ""
	if end < len(fields) {
		next = schemaEncodeCursor(end, fingerprint)
	}
	observedAt := strings.TrimSpace(value.ObservedAt)
	if observedAt == "" {
		observedAt = now().UTC().Format(time.RFC3339Nano)
	}
	source := readsource.Live(schemaParseObservedAt(observedAt, now()))
	if input.Cache {
		// The composition root replaces this with authoritative cache generation
		// metadata. This action has no generation provenance of its own, so the
		// placeholder must not claim complete, fresh coverage: if it is ever
		// surfaced unwrapped it stays honest as partial and stale.
		source = readsource.Cached(schemaParseObservedAt(observedAt, now()), readsource.CoveragePartial, "", time.Time{}, true)
	}
	return SchemaOutput{
		Status: "listed", Environment: input.Environment, Site: input.Site,
		DatasourceLUID: value.DatasourceLUID, DatasourceName: value.DatasourceName,
		Tables: append([]Table(nil), value.Tables...), Page: SchemaPage{Returned: end - offset, Total: len(fields), Limit: limit, NextCursor: next, MoreAvailable: next != ""},
		Fields: append([]Field(nil), fields[offset:end]...), Warnings: append([]string(nil), value.Warnings...),
		Source: &source, RequestID: value.RequestID,
	}, nil
}

func schemaNormalizeFieldSelection(input SchemaInput) (SchemaInput, error) {
	selected := make(map[string]struct{}, len(input.FieldIDs)+1)
	ids := input.FieldIDs
	if input.FieldID != "" {
		ids = append(append([]string(nil), ids...), input.FieldID)
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return input, schemaSchemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field identifiers cannot be empty.", nil, "Provide an exact raw Tableau field identifier for each --field-id.")
		}
		selected[id] = struct{}{}
		if len(selected) > schemaMaxAllFields {
			return input, schemaSchemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field selection exceeds the 10,000-field inventory bound.", nil, "Select at most 10,000 distinct field identifiers.")
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

func schemaFilterFields(fields []Field, input SchemaInput) ([]Field, error) {
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
			return nil, schemaSchemaError("datasource.schema.field_not_found", errs.KindUsage, input, fmt.Sprintf("Datasource field identifier %q did not match the selected schema.", id), nil, "Verify the exact --field-id and remove any conflicting --role, --table, or --query filters.")
		case matches > 1:
			return nil, schemaSchemaError("datasource.schema.field_ambiguous", errs.KindUsage, input, fmt.Sprintf("Datasource field identifier %q matched %d fields.", id, matches), nil, "Inspect the field inventory and use --table to select the intended logical table.")
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

type schemaCursor struct {
	Offset      int    `json:"offset"`
	Fingerprint string `json:"fingerprint"`
}

func schemaInputFingerprint(input SchemaInput) string {
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

func schemaEncodeCursor(offset int, fingerprint string) string {
	value, _ := json.Marshal(schemaCursor{Offset: offset, Fingerprint: fingerprint})
	return base64.RawURLEncoding.EncodeToString(value)
}

func schemaDecodeCursor(value, fingerprint string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var decoded schemaCursor
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return 0, err
	}
	if decoded.Offset < 0 || decoded.Fingerprint != fingerprint {
		return 0, fmt.Errorf("cursor does not match this datasource schema query")
	}
	return decoded.Offset, nil
}

func schemaParseObservedAt(value string, fallback time.Time) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fallback.UTC()
	}
	return parsed.UTC()
}

func schemaSchemaError(id string, kind errs.Kind, input SchemaInput, summary string, cause error, corrective string) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "datasource.schema", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: new(false), CorrectiveAction: corrective}
}

// SchemaNormalizeInput validates only caller-controlled values without dependencies.
func SchemaNormalizeInput(input SchemaInput) (SchemaInput, error) {
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.DatasourceLUID = strings.TrimSpace(input.DatasourceLUID)
	input.Query = strings.TrimSpace(input.Query)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.Table = strings.TrimSpace(input.Table)
	var err error
	input, err = schemaNormalizeFieldSelection(input)
	if err != nil {
		return input, err
	}
	if input.DatasourceLUID == "" {
		return input, schemaSchemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource schema discovery requires --id.", nil, "Provide one authoritative datasource LUID with --id.")
	}
	if input.Role != "" && input.Role != "measure" && input.Role != "dimension" && input.Role != "date" && input.Role != "excluded" {
		return input, schemaSchemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field role is invalid.", nil, "Use measure, dimension, date, or excluded.")
	}
	limit := input.Limit
	if limit == 0 {
		limit = schemaDefaultLimit
	}
	if limit < 1 || limit > schemaMaxLimit {
		return input, schemaSchemaError("datasource.schema.usage", errs.KindUsage, input, fmt.Sprintf("Datasource schema limit must be between 1 and %d.", schemaMaxLimit), nil, fmt.Sprintf("Set --limit between 1 and %d.", schemaMaxLimit))
	}
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return input, schemaSchemaError("datasource.schema.usage", errs.KindUsage, input, "--all cannot be combined with --limit or --cursor.", nil, "Use --all for the complete field inventory, or --limit for a bounded view.")
		}
		limit = schemaMaxAllFields
	}

	if input.Cursor != "" {
		var decoded schemaCursor
		raw, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(raw, &decoded) != nil || decoded.Offset < 0 || decoded.Fingerprint == "" {
			return input, schemaSchemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid.", nil, "Restart without --cursor.")
		}
	}
	return input, nil
}

// SchemaValidateContinuation checks a cursor against the locally resolved target.
func SchemaValidateContinuation(input SchemaInput) error {
	if _, err := schemaDecodeCursor(input.Cursor, schemaInputFingerprint(input)); err != nil {
		return schemaSchemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid for this query.", err, "Restart without --cursor.")
	}
	return nil
}
