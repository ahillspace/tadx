package update

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type markedVerification bool

func (e markedVerification) Error() string            { return "verification marker" }
func (e markedVerification) VerificationFailed() bool { return bool(e) }
func TestResourceFailurePhaseAndRecovery(t *testing.T) {
	for _, resource := range []struct {
		name    string
		failure func(errs.Outcome, error) error
	}{
		{"database", func(outcome errs.Outcome, cause error) error {
			return databaseFailure(DatabaseInput{Environment: "fixture", Site: "site", ID: "asset"}, "write", []string{"tags"}, outcome, cause)
		}},
		{"table", func(outcome errs.Outcome, cause error) error {
			return tableFailure(TableInput{Environment: "fixture", Site: "site", ID: "asset"}, "write", []string{"tags"}, outcome, cause)
		}},
		{"column", func(outcome errs.Outcome, cause error) error {
			return columnFailure(ColumnInput{Environment: "fixture", Site: "site", ID: "asset", TableID: "parent"}, "write", []string{"tags"}, outcome, cause)
		}},
	} {
		for _, tc := range []struct {
			name    string
			cause   error
			outcome errs.Outcome
			phase   errs.Phase
		}{
			{"unknown", errors.New("transport"), errs.OutcomeUnknown, errs.PhaseSubmission},
			{"confirmed", errors.New("transport"), errs.OutcomeConfirmed, errs.PhaseSubmission},
			{"structured", fmt.Errorf("wrapped: %w", &errs.Error{Phase: errs.PhaseVerification}), errs.OutcomeConfirmed, errs.PhaseVerification},
			{"marker", fmt.Errorf("wrapped: %w", markedVerification(true)), errs.OutcomeUnknown, errs.PhaseVerification},
			{"false-marker", markedVerification(false), errs.OutcomeUnknown, errs.PhaseSubmission},
			{"not-attempted", markedVerification(true), errs.OutcomeNotAttempted, errs.PhaseValidation},
		} {
			t.Run(resource.name+"/"+tc.name, func(t *testing.T) {
				err := resource.failure(tc.outcome, tc.cause)
				e, ok := errors.AsType[*errs.Error](err)
				if !ok || e.ID != "catalog."+resource.name+".update.write" || e.Phase != tc.phase || e.Outcome != tc.outcome || e.Environment != "fixture" || e.Site != "site" || e.Resource != "asset" || !slices.Equal(e.Completed, []string{"tags"}) || !errors.Is(err, tc.cause) {
					t.Fatalf("failure=%+v", e)
				}
				if resource.name == "column" && !strings.Contains(e.CorrectiveAction, "--table-id parent") {
					t.Fatalf("parent recovery lost: %+v", e)
				}
			})
		}
	}
}
