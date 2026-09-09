package schema

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// NormalizeInput validates only caller-controlled values without dependencies.
func NormalizeInput(input Input) (Input, error) {
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.DatasourceLUID = strings.TrimSpace(input.DatasourceLUID)
	input.Query = strings.TrimSpace(input.Query)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.Table = strings.TrimSpace(input.Table)
	var err error
	input, err = normalizeFieldSelection(input)
	if err != nil {
		return input, err
	}
	if input.DatasourceLUID == "" {
		return input, schemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource schema discovery requires --id.", nil, "Provide one authoritative datasource LUID with --id.")
	}
	if input.Role != "" && input.Role != "measure" && input.Role != "dimension" && input.Role != "date" && input.Role != "excluded" {
		return input, schemaError("datasource.schema.usage", errs.KindUsage, input, "Datasource field role is invalid.", nil, "Use measure, dimension, date, or excluded.")
	}
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return input, schemaError("datasource.schema.usage", errs.KindUsage, input, fmt.Sprintf("Datasource schema limit must be between 1 and %d.", maxLimit), nil, fmt.Sprintf("Set --limit between 1 and %d.", maxLimit))
	}
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return input, schemaError("datasource.schema.usage", errs.KindUsage, input, "--all cannot be combined with --limit or --cursor.", nil, "Use --all for the complete field inventory, or --limit for a bounded view.")
		}
		limit = maxAllFields
	}

	if input.Cursor != "" {
		var decoded cursor
		raw, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(raw, &decoded) != nil || decoded.Offset < 0 || decoded.Fingerprint == "" {
			return input, schemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid.", nil, "Restart without --cursor.")
		}
	}
	return input, nil
}

// ValidateContinuation checks a cursor against the locally resolved target.
func ValidateContinuation(input Input) error {
	if _, err := decodeCursor(input.Cursor, inputFingerprint(input)); err != nil {
		return schemaError("datasource.schema.cursor", errs.KindUsage, input, "Datasource schema cursor is invalid for this query.", err, "Restart without --cursor.")
	}
	return nil
}
