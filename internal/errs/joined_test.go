package errs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

func TestStructureRetainsIndependentJoinedFailures(t *testing.T) {
	primary := &errs.Error{ID: "publish.failed", Kind: errs.KindOperation, Summary: "Publication is uncertain.", Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	secondary := &errs.Error{ID: "publish.receipt", Kind: errs.KindOperation, Summary: "Receipt could not be saved.", Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown, CorrectiveAction: "Preserve the returned request ID."}
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"joined", errors.Join(primary, secondary)},
		{"wrapped join", fmt.Errorf("operation failed: %w", errors.Join(primary, secondary))},
		{"joined cause", &errs.Error{ID: "outer", Cause: errors.Join(primary, secondary)}},
		{"plain sibling", errors.Join(errors.New("cleanup also failed"), primary, secondary)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := errs.Structure(tc.err)
			wantPrimary := "publish.failed"
			if tc.name == "joined cause" {
				wantPrimary = "outer"
			}
			if payload.Error.ID != wantPrimary {
				t.Fatalf("primary ID=%q, want %q", payload.Error.ID, wantPrimary)
			}
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`"secondary_errors":`, `"id":"publish.receipt"`, `"phase":"persistence"`, "Preserve the returned request ID."} {
				if !strings.Contains(string(data), want) {
					t.Errorf("missing %s: %s", want, data)
				}
			}
		})
	}
}
