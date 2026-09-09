package list

import "github.com/ahillspace/tadx/internal/errs"

// ValidateInput validates bounds and continuation identity without a reader.
func ValidateInput(input Input) error {
	if input.All {
		if input.Limit != 0 || input.Cursor != "" {
			return errs.New(errs.KindUsage, "--all cannot be combined with --limit or --cursor")
		}
		return nil
	}
	fingerprint, err := datasourceFilterFingerprint(input)
	if err != nil {
		return err
	}
	_, _, _, err = pageSelection(input.Cursor, input.Limit, fingerprint)
	if err != nil {
		return errs.New(errs.KindUsage, err.Error())
	}
	return nil
}
