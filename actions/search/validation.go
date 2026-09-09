package search

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks search arguments without asserting remote target state.
func ValidateInput(input Input) error {
	input.Terms = strings.TrimSpace(input.Terms)
	if _, err := Types(input.Type); err != nil {
		return &errs.Error{ID: "search.usage", Kind: errs.KindUsage, Operation: "search", Summary: "Search type is not supported.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Use --type workbook, datasource, flow, project, user, group, definition, metric, content, admin, or pulse; omit --type to search all types."}
	}
	if (input.Terms == "" && input.Type == "") || input.Limit < 0 || input.Limit > 2000 || (input.Limit > 100 && input.Cursor != "") {
		return searchError(errs.KindUsage, input, "Search requires a term or type and a limit from 0 through 2000; legacy cursors support limits up to 100.", nil)
	}
	return nil
}

// ValidateContinuation checks cursor ownership after local target resolution.
func ValidateContinuation(input Input) error {
	if input.Limit == 0 {
		input.Limit = 20
	}
	if _, err := decodeCursor(input.Cursor, input); err != nil {
		return searchError(errs.KindUsage, input, "Search cursor does not match the selected source and filters.", err)
	}
	return nil
}
