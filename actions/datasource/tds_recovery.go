package datasource

import (
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type TDSAttemptEntry struct {
	value.TDSAttempt
	Outcome string   `json:"submission_outcome"`
	Help    []string `json:"help,omitempty"`
}
type TDSAttemptsOutput struct {
	Attempts []TDSAttemptEntry `json:"attempts"`
	More     bool              `json:"more_available"`
	Details  string            `json:"details,omitempty"`
}

func DescribeTDSAttempts(environment string, records []value.TDSAttempt, more bool) TDSAttemptsOutput {
	out := TDSAttemptsOutput{Attempts: make([]TDSAttemptEntry, 0, len(records)), More: more}
	for _, a := range records {
		row := TDSAttemptEntry{TDSAttempt: a, Outcome: string(submitOutcome(a.Status))}
		if a.WorkCopyLUID != "" {
			row.Help = append(row.Help, commandhint.Environment(environment, "content", "datasource", "work-copy", "--work-copy", a.WorkCopyLUID))
		}
		if a.DestinationDigest != "" && (a.PublishedLUID != "" || a.Operation == "datasource.save.overwrite") {
			row.Help = append(row.Help, commandhint.Environment(environment, "content", "datasource", "work-copy", "--attempt", a.ID, "--verify"))
		}
		if a.Status == "pending" || a.Status == "unknown" {
			row.Help = append(row.Help, "The earlier request may have reached Tableau. Review its outcome before acknowledging a new submission.")
		}
		out.Attempts = append(out.Attempts, row)
	}
	return out
}

func (o TDSAttemptsOutput) CompactOutput() any {
	type entry struct {
		ProvenanceOnly bool     `json:"checkout_provenance_only,omitempty"`
		ID             string   `json:"attempt_id,omitempty"`
		Operation      string   `json:"operation"`
		Status         string   `json:"submit_status"`
		Outcome        string   `json:"submission_outcome"`
		Source         string   `json:"source_luid,omitempty"`
		Copy           string   `json:"work_copy_luid,omitempty"`
		Published      string   `json:"published_luid,omitempty"`
		Reported       string   `json:"provider_reported_luid,omitempty"`
		Unexpected     string   `json:"additional_provider_reported_luid,omitempty"`
		Verification   string   `json:"verification,omitempty"`
		SupersededBy   string   `json:"acknowledged_by_attempt,omitempty"`
		Help           []string `json:"help,omitempty"`
	}
	n := min(len(o.Attempts), 10)
	compact := make([]entry, n)
	for i, r := range o.Attempts[:n] {
		compact[i] = entry{r.ProvenanceOnly, r.ID, r.Operation, r.Status, r.Outcome, r.SourceLUID, r.WorkCopyLUID, r.PublishedLUID, r.ReportedLUID, r.UnexpectedIdentity, r.Verification, r.SupersededBy, r.Help}
	}
	return struct {
		Attempts []entry `json:"attempts"`
		More     bool    `json:"more_available"`
		Details  string  `json:"details"`
	}{compact, o.More || n < len(o.Attempts), "--full"}
}
func (o TDSAttemptsOutput) FullOutput() any { return o }

// CheckRetainedSource validates the historical fields this operation will
// consume against the credentials of the current authenticated session.
func CheckRetainedSource(operation, copyID string, redact func(string) string, fields ...string) error {
	if err := checkLifecycleMetadata(redact, fields...); err != nil {
		return lifecycleFailure(operation, copyID, "Retained checkout provenance contains protected credential material.", errs.PhaseValidation, errs.OutcomeNotAttempted, err)
	}
	return nil
}

// CheckRetainedAttempt protects provider-owned strings when a different PAT for
// the same Tableau caller reopens this private journal.
func CheckRetainedAttempt(a value.TDSAttempt, redact func(string) string) error {
	if err := checkLifecycleMetadata(redact, a.ID, a.Scope.Server, a.Scope.SiteLUID, a.Scope.CallerLUID, a.SourceLUID, a.WorkCopyLUID, a.PublishedLUID, a.UnexpectedIdentity, a.ReportedLUID, a.RequestID, a.SourceRevision, a.SupersededBy); err != nil {
		return err
	}
	if a.Checkout != nil {
		if err := checkLifecycleMetadata(redact, a.Checkout.SourceName, a.Checkout.SourceProjectLUID, a.Checkout.Eligibility.RESTVersion); err != nil {
			return err
		}
	}
	for _, p := range a.Projections {
		if err := checkLifecycleMetadata(redact, p.Name); err != nil {
			return err
		}
	}
	return nil
}
