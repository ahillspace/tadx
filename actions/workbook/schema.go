package workbook

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// SchemaReader supplies normalized fields and their exact workbook provenance.
type SchemaReader interface {
	ReadWorkbookSchema(context.Context, string) (value.WorkbookSchema, error)
}

type SchemaInput struct {
	Environment, Site, WorkbookLUID, Datasource, Query, Role, Cursor string
	FieldIDs                                                         []string
	Limit                                                            int
	All, Descriptions, Tags                                          bool
}

type SchemaPage struct {
	Returned      int    `json:"returned"`
	Total         int    `json:"total"`
	TotalComplete bool   `json:"total_complete"`
	Limit         int    `json:"limit"`
	NextCursor    string `json:"next_cursor,omitempty"`
	MoreAvailable bool   `json:"more_available"`
}

type SchemaGroup struct {
	Identity value.MetadataIdentity   `json:"identity"`
	Kind     string                   `json:"kind"`
	Parents  []value.MetadataIdentity `json:"parents,omitempty"`
	Fields   []value.SchemaField      `json:"fields"`
}

type SchemaOutput struct {
	Status      string                 `json:"status"`
	Environment string                 `json:"environment,omitempty"`
	Site        string                 `json:"site,omitempty"`
	Workbook    value.MetadataIdentity `json:"workbook"`
	Complete    bool                   `json:"complete"`
	Datasources []SchemaGroup          `json:"datasources"`
	Page        SchemaPage             `json:"page"`
	Warnings    []string               `json:"warnings,omitempty"`
	RequestID   string                 `json:"tableau_request_id,omitempty"`
}

func (o SchemaOutput) CompactOutput() any {
	type group struct {
		Identity value.MetadataIdentity     `json:"identity"`
		Kind     string                     `json:"kind"`
		Parents  []value.MetadataIdentity   `json:"parents,omitempty"`
		Fields   []value.SchemaCompactField `json:"fields"`
	}
	groups := make([]group, len(o.Datasources))
	for i, ds := range o.Datasources {
		groups[i] = group{Identity: ds.Identity, Kind: ds.Kind, Parents: ds.Parents, Fields: value.CompactSchemaFields(ds.Fields)}
	}
	return struct {
		Status      string                 `json:"status"`
		Environment string                 `json:"environment,omitempty"`
		Site        string                 `json:"site,omitempty"`
		Workbook    value.MetadataIdentity `json:"workbook"`
		Complete    bool                   `json:"complete"`
		Datasources []group                `json:"datasources"`
		Page        SchemaPage             `json:"page"`
		Warnings    []string               `json:"warnings,omitempty"`
		Details     string                 `json:"details"`
	}{o.Status, o.Environment, o.Site, o.Workbook, o.Complete, groups, o.Page, o.Warnings, "--full"}
}
func (o SchemaOutput) FullOutput() any { return o }

func schemaError(in SchemaInput, kind errs.Kind, message string, cause error) error {
	return &errs.Error{ID: "workbook.schema." + string(kind), Kind: kind, Operation: "workbook.schema", Environment: in.Environment, Site: in.Site, Summary: message, Cause: cause, Retryable: new(false), CorrectiveAction: "Inspect the workbook schema and correct the exact selectors; restart without --cursor if the schema changed.", TableauRequestID: errs.TableauRequestID(cause)}
}

func schemaReadError(in SchemaInput, message string, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Verify workbook and datasource access and Metadata API availability, then retry.")
	return &errs.Error{ID: "workbook.schema.read", Kind: errs.KindOperation, Operation: "workbook.schema", Environment: in.Environment, Site: in.Site, Summary: message, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause)}
}

func ValidateSchemaInput(in SchemaInput) error {
	if strings.TrimSpace(in.WorkbookLUID) == "" || in.WorkbookLUID != strings.TrimSpace(in.WorkbookLUID) {
		return schemaError(in, errs.KindUsage, "Workbook schema requires one exact --id.", nil)
	}
	if in.Limit < 0 || in.Limit > 10000 || (in.All && (in.Limit != 0 || in.Cursor != "")) {
		return schemaError(in, errs.KindUsage, "Use --limit from 1 to 10000, or --all without --limit or --cursor.", nil)
	}
	if !slices.Contains([]string{"", "measure", "dimension", "date", "excluded"}, strings.ToLower(strings.TrimSpace(in.Role))) {
		return schemaError(in, errs.KindUsage, "Use field role measure, dimension, date, or excluded.", nil)
	}
	if len(in.FieldIDs) > 10000 {
		return schemaError(in, errs.KindUsage, "Select at most 10000 field identifiers.", nil)
	}
	for _, id := range in.FieldIDs {
		if strings.TrimSpace(id) == "" {
			return schemaError(in, errs.KindUsage, "Field identifiers must not be empty.", nil)
		}
	}
	if in.Cursor != "" {
		var cursor struct {
			Offset      int
			Fingerprint string
		}
		raw, err := base64.RawURLEncoding.DecodeString(in.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Offset < 0 || cursor.Fingerprint == "" {
			return schemaError(in, errs.KindUsage, "Invalid schema cursor.", nil)
		}
	}
	return nil
}

// Schema applies one field window across the workbook's datasource groups.
func Schema(ctx context.Context, reader SchemaReader, in SchemaInput) (SchemaOutput, error) {
	if err := ValidateSchemaInput(in); err != nil {
		return SchemaOutput{}, err
	}
	if reader == nil {
		return SchemaOutput{}, schemaError(in, errs.KindRuntime, "Workbook schema is not configured.", nil)
	}
	record, readErr := reader.ReadWorkbookSchema(ctx, in.WorkbookLUID)
	m := record.Metadata
	out := SchemaOutput{Status: "listed", Environment: in.Environment, Site: in.Site, Workbook: m.Identity, Complete: m.Complete, Warnings: slices.Clone(m.Warnings), RequestID: m.TableauRequestID, Datasources: []SchemaGroup{}}
	if m.Identity.LUID != in.WorkbookLUID || m.Identity.MetadataID == "" || len(record.Fields) != len(m.Datasources) {
		out.Status, out.Complete = "partial", false
		if readErr != nil {
			return out, schemaReadError(in, "Workbook schema discovery failed.", readErr)
		}
		return out, schemaError(in, errs.KindOperation, "Workbook schema identity or datasource grouping is incomplete.", readErr)
	}
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.Query = strings.TrimSpace(in.Query)
	inventoryComplete := m.Complete && readErr == nil
	for _, ds := range m.Datasources {
		inventoryComplete = inventoryComplete && ds.Complete
	}
	selectionIncomplete := !inventoryComplete && (in.Datasource != "" || len(in.FieldIDs) > 0)
	type entry struct {
		Group int
		Field value.SchemaField
	}
	var selected []entry
	groupMatches := 0
	counts := make(map[string]int)
	for _, id := range in.FieldIDs {
		counts[id] = 0
	}
	for g, ds := range m.Datasources {
		if in.Datasource != "" && ds.Identity.Name != in.Datasource {
			continue
		}
		groupMatches++
		out.Complete = out.Complete && ds.Complete
		out.Warnings = append(out.Warnings, ds.Warnings...)
		for _, f := range record.Fields[g] {
			if !f.Matches(in.Query, in.Role, "") {
				continue
			}
			if len(counts) > 0 {
				if _, ok := counts[f.ID]; !ok {
					continue
				}
				counts[f.ID]++
			}
			selected = append(selected, entry{g, f})
		}
	}
	if in.Datasource != "" && (groupMatches > 1 || groupMatches == 0 && inventoryComplete) {
		return out, schemaError(in, errs.KindUsage, "Datasource name must match exactly one datasource inside the workbook.", nil)
	}
	for id, n := range counts {
		if n > 1 || n == 0 && inventoryComplete {
			return out, schemaError(in, errs.KindUsage, fmt.Sprintf("Field %q matched %d fields; use --datasource and matching filters to select one.", id, n), nil)
		}
	}
	slices.SortFunc(selected, func(a, b entry) int {
		if c := strings.Compare(m.Datasources[a.Group].Identity.MetadataID, m.Datasources[b.Group].Identity.MetadataID); c != 0 {
			return c
		}
		if c := strings.Compare(a.Field.Table, b.Field.Table); c != 0 {
			return c
		}
		if c := strings.Compare(a.Field.Caption, b.Field.Caption); c != 0 {
			return c
		}
		return strings.Compare(a.Field.ID, b.Field.ID)
	})
	request := in
	request.Cursor = ""
	request.Limit = 0
	request.FieldIDs = slices.Sorted(slices.Values(in.FieldIDs))
	fingerprintData, _ := json.Marshal(struct {
		Request     SchemaInput
		Datasources []value.MetadataWorkbookDatasource
		Selected    []entry
	}{request, m.Datasources, selected})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(fingerprintData))
	cursor := struct {
		Offset      int
		Fingerprint string
	}{Fingerprint: fingerprint}
	if in.Cursor != "" {
		raw, _ := base64.RawURLEncoding.DecodeString(in.Cursor)
		_ = json.Unmarshal(raw, &cursor)
		if cursor.Fingerprint != fingerprint || cursor.Offset > len(selected) {
			return out, schemaError(in, errs.KindUsage, "Schema cursor does not match the current workbook, filters, or observed schema.", nil)
		}
	}
	offset := cursor.Offset
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	if in.All {
		limit = 10000
	}
	if in.All && len(selected) > limit {
		return out, schemaError(in, errs.KindOperation, "Matching schema exceeds the 10000-field bound; narrow the selection.", nil)
	}
	end := min(cursor.Offset+limit, len(selected))
	out.Page = SchemaPage{Returned: end - cursor.Offset, Total: len(selected), TotalComplete: out.Complete && readErr == nil, Limit: limit, MoreAvailable: end < len(selected)}
	if out.Page.MoreAvailable {
		cursor.Offset = end
		raw, _ := json.Marshal(cursor)
		out.Page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	lastGroup := -1
	for _, item := range selected[offset:end] {
		if item.Group != lastGroup {
			ds := m.Datasources[item.Group]
			out.Datasources = append(out.Datasources, SchemaGroup{Identity: ds.Identity, Kind: ds.Kind, Parents: slices.Clone(ds.Parents), Fields: []value.SchemaField{}})
			lastGroup = item.Group
		}
		f := item.Field.ProjectMetadata(in.Descriptions, in.Tags, true)
		i := len(out.Datasources) - 1
		out.Datasources[i].Fields = append(out.Datasources[i].Fields, f)
	}
	if readErr != nil || selectionIncomplete {
		out.Complete = false
		out.Status = "partial"
		if selectionIncomplete {
			out.Warnings = append(out.Warnings, "Incomplete metadata cannot establish exact selector absence or uniqueness; returned fields are confirmed matches from the observed inventory.")
		}
		if readErr != nil {
			return out, schemaReadError(in, "Workbook schema metadata is incomplete.", readErr)
		}
		return out, schemaError(in, errs.KindOperation, "Workbook schema metadata is incomplete.", nil)
	}
	if !out.Complete {
		out.Status = "partial"
	}
	return out, nil
}
