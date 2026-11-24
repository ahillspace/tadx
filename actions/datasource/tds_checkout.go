package datasource

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

type WorkCopyCreator interface {
	CreateWorkCopy(context.Context, string) (value.TDSReceipt, error)
}

// A revision must cover the full published definition. An updated-at timestamp
// without that guarantee cannot authorize overwriting the original.
type SourceRevisionReader interface {
	ReadTDSRevision(context.Context, string) (string, error)
}

type TDSJournal interface {
	LockLifecycle(context.Context, value.TDSAttempt, []string) (func() error, []value.TDSAcknowledgement, error)
	Begin(context.Context, value.TDSAttempt, []string) (value.TDSAttempt, error)
	Record(context.Context, value.TDSAttempt) error
	Origin(context.Context, value.TDSScope, string) (value.TDSAttempt, error)
}

type CheckoutInput struct {
	Eligibility        value.TDSEligibility
	Scope              value.TDSScope
	Environment        string
	Selector           identity.Selector
	AcknowledgeUnknown []string
	AttemptOutput      io.Writer
}

type LifecycleOutput struct {
	Eligibility           value.TDSEligibility       `json:"eligibility,omitzero"`
	Checkout              *value.TDSCheckoutContext  `json:"checkout_receipt,omitempty"`
	Files                 *EditExports               `json:"files,omitempty"`
	ObservationsComplete  *bool                      `json:"observations_complete,omitempty"`
	MetadataWarning       string                     `json:"metadata_warning,omitempty"`
	ExportHelp            []string                   `json:"export_help,omitempty"`
	Operation             string                     `json:"operation"`
	Mode                  string                     `json:"mode"`
	Status                string                     `json:"status"`
	Environment           string                     `json:"environment,omitempty"`
	SiteLUID              string                     `json:"site_luid,omitempty"`
	SourceLUID            string                     `json:"source_luid,omitempty"`
	WorkCopyLUID          string                     `json:"work_copy_luid,omitempty"`
	PublishedLUID         string                     `json:"published_luid,omitempty"`
	UnexpectedIdentity    string                     `json:"additional_provider_reported_luid,omitempty"`
	ReportedLUID          string                     `json:"provider_reported_luid,omitempty"`
	Name                  string                     `json:"name,omitempty"`
	ProjectLUID           string                     `json:"project_luid,omitempty"`
	Description           *string                    `json:"requested_description,omitempty"`
	PublishedName         string                     `json:"published_name,omitempty"`
	PublishedProjectLUID  string                     `json:"published_project_luid,omitempty"`
	DestinationMatch      *bool                      `json:"destination_matches,omitempty"`
	WorkCopyAfterSave     string                     `json:"work_copy_after_save,omitempty"`
	AttemptID             string                     `json:"attempt_id,omitempty"`
	Acknowledges          []string                   `json:"acknowledges_unknown_attempts,omitempty"`
	AcknowledgementScopes []value.TDSAcknowledgement `json:"acknowledgement_scopes,omitempty"`
	RequestID             string                     `json:"tableau_request_id,omitempty"`
	PreviewToken          string                     `json:"preview_token,omitempty"`
	RevisionKnown         bool                       `json:"source_revision_known"`
	Verification          string                     `json:"verification,omitempty"`
	Coverage              string                     `json:"verification_scope,omitempty"`
	Observations          []SaveObservation          `json:"observations,omitempty"`
	OmittedObservations   int                        `json:"omitted_observations,omitempty"`
	Warnings              []string                   `json:"warnings,omitempty"`
	Details               string                     `json:"details,omitempty"`
	Help                  []string                   `json:"help,omitempty"`
}

func lifecycleFailure(op, resource, summary string, phase errs.Phase, outcome errs.Outcome, cause error) error {
	return &errs.Error{ID: op + ".failed", Kind: errs.KindOperation, Operation: op, Resource: resource, Phase: phase, Outcome: outcome, Summary: summary, Retryable: new(false), Cause: cause, TableauRequestID: errs.TableauRequestID(cause)}
}

// Provider diagnostics are protected at the transport. Resolution and admission
// can build new messages from candidate LUIDs and historical attempt IDs.
func lifecycleCause(err error, redact func(string) string) error {
	if err == nil || redact == nil {
		return err
	}
	message := err.Error()
	if safe := redact(message); safe != message {
		return &lifecycleDiagnostic{cause: err, message: safe}
	}
	return err
}

type lifecycleDiagnostic struct {
	cause   error
	message string
}

func (e *lifecycleDiagnostic) Error() string { return e.message }
func (e *lifecycleDiagnostic) Unwrap() error { return e.cause }

func ValidateCheckoutInput(in CheckoutInput) error {
	s := in.Selector
	if s.LUID != "" {
		if !value.ValidTDSLUID(string(s.LUID)) || s.Name != "" || s.ProjectPath != "" || s.ProjectLUID != "" {
			return editUsage("Checkout requires one datasource UUID or an exact name and project.")
		}
	} else if strings.TrimSpace(s.Name) == "" || (s.ProjectPath == "") == (s.ProjectLUID == "") || (s.ProjectLUID != "" && !value.ValidTDSLUID(string(s.ProjectLUID))) {
		return editUsage("Checkout by name requires one exact project path or project UUID.")
	}
	return nil
}

func lifecycleDigest(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	} // Only typed strings, booleans and ordered projections are hashed.
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

func Checkout(ctx context.Context, resolver Resolver, creator WorkCopyCreator, journal TDSJournal, in CheckoutInput, preview bool) (LifecycleOutput, error) {
	const op = "datasource.checkout"
	out := LifecycleOutput{Eligibility: in.Eligibility, Operation: op, Mode: "execute", Status: "not_attempted", Environment: in.Environment, SiteLUID: in.Scope.SiteLUID, Acknowledges: in.AcknowledgeUnknown}
	if preview {
		out.Mode = "preview"
	}
	if err := ValidateCheckoutInput(in); err != nil {
		return out, err
	}
	var redact func(string) string
	if provider, ok := creator.(interface{ TDSRedactor() func(string) string }); ok {
		redact = provider.TDSRedactor()
	}
	source, err := resolver.ResolveDatasource(ctx, in.Selector)
	if err != nil {
		return out, lifecycleFailure(op, "", "Could not resolve the checkout source.", errs.PhaseSetup, errs.OutcomeNotAttempted, lifecycleCause(err, redact))
	}
	if !value.ValidTDSLUID(source.LUID) || (in.Selector.LUID != "" && !strings.EqualFold(source.LUID, string(in.Selector.LUID))) {
		return out, lifecycleFailure(op, "", "Tableau did not establish the requested datasource identity.", errs.PhaseSetup, errs.OutcomeNotAttempted, nil)
	}
	if err := checkLifecycleMetadata(redact, source.LUID, source.Name, source.ProjectLUID, source.UpdatedAt); err != nil {
		return out, lifecycleFailure(op, "", "Checkout source metadata contains protected credential material.", errs.PhaseValidation, errs.OutcomeNotAttempted, nil)
	}
	out.SourceLUID, out.Name, out.ProjectLUID = source.LUID, source.Name, source.ProjectLUID
	if in.Eligibility.RESTStatus == "unsupported" || in.Eligibility.RESTStatus == "unknown" {
		return out, lifecycleFailure(op, source.LUID, "Checkout requires Tableau REST API 3.30 or later.", errs.PhaseSetup, errs.OutcomeNotAttempted, nil)
	}
	draft := value.TDSAttempt{Checkout: &value.TDSCheckoutContext{Eligibility: in.Eligibility, SourceName: source.Name, SourceProjectLUID: source.ProjectLUID, SourceObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}, Scope: in.Scope, SourceLUID: source.LUID, Operation: op, IntentDigest: lifecycleDigest([]string{op, source.LUID})}
	release, matches, err := journal.LockLifecycle(ctx, draft, in.AcknowledgeUnknown)
	if err != nil {
		return out, lifecycleFailure(op, source.LUID, "Checkout admission failed.", errs.PhaseSetup, errs.OutcomeNotAttempted, lifecycleCause(err, redact))
	}
	defer release()
	if err := checkAcknowledgementScopes(matches, redact); err != nil {
		return out, lifecycleFailure(op, source.LUID, "Acknowledgement scopes contain protected credential material.", errs.PhaseValidation, errs.OutcomeNotAttempted, err)
	}
	out.AcknowledgementScopes = matches
	if len(in.AcknowledgeUnknown) > 0 {
		out.Warnings = append(out.Warnings, "The earlier checkout may still take effect. A fresh request can allocate another work copy.")
	}
	revisions, hasRevisions := resolver.(SourceRevisionReader)
	if hasRevisions {
		draft.SourceRevision, err = revisions.ReadTDSRevision(ctx, source.LUID)
		if err != nil || checkLifecycleMetadata(redact, draft.SourceRevision) != nil {
			draft.SourceRevision = ""
		}
	}
	out.RevisionKnown = draft.SourceRevision != ""
	if preview {
		out.Status = "preview"
		return out, nil
	}
	// A checkout response must be durable immediately. Establish the baseline
	// separately after re-reading the source, never by trusting the pre-read.
	before := draft.SourceRevision
	draft.SourceRevision = ""
	a, err := journal.Begin(ctx, draft, in.AcknowledgeUnknown)
	if err != nil {
		out.AttemptID = a.ID
		out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "work-copy", "--attempts", "--id", source.LUID)}
		return out, lifecycleFailure(op, source.LUID, "Could not retain the checkout attempt.", errs.PhasePersistence, errs.OutcomeNotAttempted, lifecycleCause(err, redact))
	}
	out.AttemptID = a.ID
	out.RevisionKnown = false
	out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "work-copy", "--attempt", a.ID)}
	if err := errors.Join(ctx.Err(), emitTDSAttempt(in.AttemptOutput, a.ID)); err != nil {
		return out, abortUndispatched(ctx, journal, a, err)
	}
	receipt, submitErr := creator.CreateWorkCopy(ctx, source.LUID)
	a.RequestID = receipt.TableauRequestID
	a.Status = receiptStatus(receipt, submitErr)
	if receipt.Eligibility.Operation != "" {
		out.Eligibility = receipt.Eligibility
	}
	if a.Status == "acknowledged" && (!value.ValidTDSLUID(receipt.WorkCopyLUID) || strings.EqualFold(receipt.WorkCopyLUID, source.LUID) || receipt.DatasourceLUID != "") {
		a.Status = "unknown"
		submitErr = errors.New("checkout response contains an invalid or contradictory identity")
		out.Eligibility = in.Eligibility
		if out.Eligibility.Operation != "" {
			out.Eligibility.Reason = "identity_unconfirmed"
		}
	}
	a.Checkout.Eligibility = out.Eligibility
	if a.Status == "acknowledged" {
		a.WorkCopyLUID = receipt.WorkCopyLUID
		a.Checkout.AcknowledgedAt = time.Now().UTC().Format(time.RFC3339Nano)
		out.Checkout = a.Checkout
	} else {
		a.ReportedLUID = receipt.WorkCopyLUID
		a.UnexpectedIdentity = receipt.DatasourceLUID
		out.UnexpectedIdentity = a.UnexpectedIdentity
	}
	out.WorkCopyLUID, out.ReportedLUID, out.RequestID, out.Status = a.WorkCopyLUID, a.ReportedLUID, a.RequestID, a.Status
	if err := recordTDSReceipt(ctx, journal, a); err != nil {
		return out, lifecycleFailure(op, source.LUID, "Could not retain the checkout response.", errs.PhasePersistence, submitOutcome(a.Status), errors.Join(submitErr, err))
	}
	if submitErr != nil || a.Status != "acknowledged" {
		return out, lifecycleFailure(op, source.LUID, "Tableau checkout did not return a confirmed work copy.", errs.PhaseSubmission, submitOutcome(a.Status), submitErr)
	}
	out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "work-copy", "--work-copy", a.WorkCopyLUID)}
	out.RevisionKnown = false
	if before != "" {
		after, err := revisions.ReadTDSRevision(ctx, source.LUID)
		if err == nil && before == after {
			a.SourceRevision = after
			if err := recordTDSReceipt(ctx, journal, a); err != nil {
				return out, lifecycleFailure(op, a.WorkCopyLUID, "Could not retain the verified source revision.", errs.PhasePersistence, errs.OutcomeConfirmed, err)
			}
			out.RevisionKnown = true
		}
	}
	if !out.RevisionKnown {
		out.Warnings = append(out.Warnings, "The source revision is unknown. This receipt cannot authorize an overwrite.")
	}
	return out, nil
}

func emitTDSAttempt(w io.Writer, id string) error {
	if w == nil {
		return nil
	}
	_, err := fmt.Fprintln(w, "TDS attempt", id)
	return err
}

// Persist a known remote outcome even if the request context was cancelled.
func recordTDSReceipt(ctx context.Context, journal TDSJournal, a value.TDSAttempt) error {
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return journal.Record(persist, a)
}

func abortUndispatched(ctx context.Context, journal TDSJournal, a value.TDSAttempt, cause error) error {
	a.Status = "not_attempted"
	err := recordTDSReceipt(ctx, journal, a)
	return lifecycleFailure(a.Operation, a.ID, "Submission stopped before dispatch.", errs.PhasePersistence, errs.OutcomeNotAttempted, errors.Join(cause, err))
}
func receiptStatus(r value.TDSReceipt, err error) string {
	switch r.Status {
	case "acknowledged":
		if err == nil {
			return "acknowledged"
		}
	case "rejected":
		return "not_applied"
	case "not_attempted":
		return "not_attempted"
	}
	return "unknown"
}
func submitOutcome(status string) errs.Outcome {
	switch status {
	case "acknowledged":
		return errs.OutcomeConfirmed
	case "not_applied":
		return errs.OutcomeNotApplied
	case "not_attempted":
		return errs.OutcomeNotAttempted
	}
	return errs.OutcomeUnknown
}

// Exact verification values stay in the full result. Compact output preserves
// the preview settings and every nonmatching observation, even after truncation.
func (o LifecycleOutput) CompactOutput() any {
	o.RequestID = ""
	if len(o.Observations) > 10 {
		kept := make([]SaveObservation, 0, 10)
		for i, row := range o.Observations {
			if i < 10 || row.Status == "mismatch" || row.Status == "unknown" {
				kept = append(kept, row)
			}
		}
		o.OmittedObservations = len(o.Observations) - len(kept)
		o.Observations = kept
	}
	o.Details = "--full"
	return o
}
func (o LifecycleOutput) FullOutput() any { return o }

// A receipt or result must never persist protected provider metadata, even
// when the native datasource read itself completed successfully.
func checkLifecycleMetadata(redact func(string) string, values ...string) error {
	if redact != nil {
		for _, v := range values {
			if v != "" && redact(v) != v {
				return errors.New("provider metadata contains protected credential material")
			}
		}
	}
	return nil
}

func checkAcknowledgementScopes(matches []value.TDSAcknowledgement, redact func(string) string) error {
	for _, m := range matches {
		if err := checkLifecycleMetadata(redact, m.AttemptID); err != nil {
			return err
		}
		for _, guard := range m.Guards {
			if err := checkLifecycleMetadata(redact, guard.ResourceLUID, guard.CallerLUID); err != nil {
				return err
			}
		}
	}
	return nil
}
