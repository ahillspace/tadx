package list

import (
	"encoding/base64"
	"encoding/json"
	"github.com/ahillspace/tadx/internal/errs"
)

// ValidateInput checks bounded list inputs; resolved cursor ownership is checked later.
func ValidateInput(input Input) error {
	if input.Limit < 0 || input.Limit > maxLimit {
		return listError("pulse.definition.list.usage", errs.KindUsage, input, "Pulse definition list limit must be between 1 and 10000.", nil)
	}
	if input.All && (input.Limit != 0 || input.Cursor != "") {
		return listError("pulse.definition.list.usage", errs.KindUsage, input, "--all cannot be combined with --limit or --cursor; remove --limit and --cursor for all rows, or remove --all for a bounded result.", nil)
	}
	if input.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		var value cursorValue
		if len(input.Cursor) > maxCursorLength || err != nil || json.Unmarshal(data, &value) != nil || value.Version != cursorVersion || value.PageToken == "" || value.Fingerprint == "" {
			return listError("pulse.definition.list.usage", errs.KindUsage, input, "Invalid Pulse definition cursor.", nil)
		}
	}
	return nil
}

// ValidateContinuation binds a cursor to a locally resolved target before sign-in.
func ValidateContinuation(input Input) error {
	limit := input.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	fingerprint := targetFingerprint(input.Environment, input.Site, input.Name, limit, input.Cache)
	if input.DatasourceLUID != "" {
		fingerprint = targetFingerprint(fingerprint, input.DatasourceLUID, "", limit, input.Cache)
	}
	if _, err := decodeCursor(input.Cursor, fingerprint); err != nil {
		return listError("pulse.definition.list.usage", errs.KindUsage, input, "Pulse definition cursor does not match the selected target, name, and limit.", err)
	}
	return nil
}
