package list

import (
	"encoding/base64"
	"encoding/json"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks bounded list inputs; resolved cursor ownership is checked later.
func ValidateInput(input Input) error {
	if strings.TrimSpace(input.DefinitionLUID) == "" {
		return fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric list requires an exact definition LUID.", nil)
	}
	if input.Limit < 0 || input.Limit > 10000 {
		return fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric list limit must be between 1 and 10000.", nil)
	}
	if input.All && (input.Limit != 0 || input.Cursor != "") {
		return fail("pulse.metric.list.usage", errs.KindUsage, input, "--all cannot be combined with --limit or --cursor.", nil)
	}
	if input.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		var value cursor
		if len(input.Cursor) > 4096 || err != nil || json.Unmarshal(data, &value) != nil || value.Version != 1 || value.Token == "" || value.Definition != strings.TrimSpace(input.DefinitionLUID) || value.Limit < 1 || value.Limit > 10000 {
			return fail("pulse.metric.list.usage", errs.KindUsage, input, "Invalid Pulse metric cursor.", nil)
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
	if _, err := decodeCursor(input.Cursor, input.Environment, input.Site, strings.TrimSpace(input.DefinitionLUID), limit, input.Cache); err != nil {
		return fail("pulse.metric.list.usage", errs.KindUsage, input, "Pulse metric cursor does not match this definition, limit, and source.", err)
	}
	return nil
}
