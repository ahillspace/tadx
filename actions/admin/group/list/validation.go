package list

import (
	"encoding/base64"
	"encoding/json"
	"github.com/ahillspace/tadx/internal/errs"
)

// ValidateInput checks pagination shape; resolved-target cursor binding remains in Execute.
func ValidateInput(input Input) error {
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return nil
	}
	if input.Limit < 0 || input.Limit > 10000 {
		return errs.New(errs.KindUsage, "admin group list limit must be between 1 and 10000")
	}
	if input.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		var cursor cursorValue
		if len(input.Cursor) > 2048 || err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Version != 1 || cursor.Page < 2 || cursor.Size < 1 || cursor.Size > 100 || len(cursor.Snapshot) > 1024 || input.Limit != 0 && input.Limit != cursor.Size {
			return errs.New(errs.KindUsage, "invalid admin group list continuation cursor")
		}
	}
	return nil
}

// ValidateContinuation binds a private cursor after local environment resolution, before authentication.
func ValidateContinuation(input Input) error {
	fingerprint, err := cursorFingerprint(struct {
		Environment, Site, Name, Domain string
		Catalog                         bool
	}{input.Environment, input.Site, input.Name, input.Domain, input.Catalog})

	if err != nil {
		return err
	}
	_, _, _, err = selectPage(input.Cursor, input.Limit, fingerprint)
	return err
}
