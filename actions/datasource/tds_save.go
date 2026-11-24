package datasource

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/value"
)

// A TDSSaveProvider can be bound only after evidence establishes the mode's
// destination and dispatch guarantees. Native artifact publishing uses its own
// contract; its overwrite flag cannot implement a work-copy save.
type TDSSaveProvider interface {
	Resolver
	CollisionReader
	TDSReader
	TDSSaveDestination(ctx context.Context, mode, copyID, sourceID, name string) (TDSDestination, error)
	SubmitTDSSave(ctx context.Context, mode, copyID string, d TDSDestination, description *string) (value.TDSReceipt, error)
}

type TDSDestination struct {
	Name           string `json:"name"`
	ProjectLUID    string `json:"project_luid"`
	SourceRevision string `json:"source_revision,omitempty"`
}
type SaveWorkCopyInput struct {
	Redact                                func(string) string
	Scope                                 value.TDSScope
	Environment, WorkCopyLUID, Mode, Name string
	Description                           *string
	FromChanges                           bool
	Changes                               []EditChange
	Fields                                []string
	ExpectPreview                         string
	AcknowledgeUnknown                    []string
	AttemptOutput                         io.Writer
}
type SaveObservation struct {
	Selector EditSelector `json:"selector"`
	State    EditState    `json:"observed"`
	Expected *EditState   `json:"expected,omitempty"`
	Status   string       `json:"status"`
}

func ValidateSaveWorkCopyInput(in SaveWorkCopyInput) error {
	if !value.ValidTDSLUID(in.WorkCopyLUID) {
		return editUsage("Save requires one work-copy UUID.")
	}
	switch in.Mode {
	case "overwrite":
		if in.Name != "" || in.Description != nil {
			return editUsage("Overwrite takes its destination from verified checkout provenance.")
		}
	case "as-new":
		if strings.TrimSpace(in.Name) == "" {
			return editUsage("Save as new requires a nonblank name.")
		}
	default:
		return editUsage("Choose overwrite or as-new.")
	}
	if in.FromChanges {
		if len(in.Fields) > 0 {
			return editUsage("A changes file selects its own verification scope.")
		}
		return ValidateEditChanges(in.Changes)
	}
	if len(in.Changes) > 0 || len(in.Fields) > maxEditCount {
		return editUsage("Save accepts at most 50 exact field selectors or one changes file.")
	}
	seen := map[string]bool{}
	for _, f := range in.Fields {
		if strings.TrimSpace(f) == "" || seen[f] {
			return editUsage("Save requires distinct, nonblank field selectors.")
		}
		seen[f] = true
	}
	return nil
}

func SaveWorkCopy(ctx context.Context, p TDSSaveProvider, j TDSJournal, in SaveWorkCopyInput, preview bool) (LifecycleOutput, error) {
	op := "datasource.save." + in.Mode
	out := LifecycleOutput{Operation: op, Mode: "preview", Status: "not_attempted", WorkCopyLUID: in.WorkCopyLUID, Environment: in.Environment, SiteLUID: in.Scope.SiteLUID, Acknowledges: in.AcknowledgeUnknown, Description: in.Description, Warnings: []string{"Tableau publishes the entire work copy. Verification covers only the reported scope."}}
	if err := ValidateSaveWorkCopyInput(in); err != nil {
		return out, err
	}
	if p == nil {
		reason := "Save as new requires verified destination placement and create-only behavior under a concurrent name collision."
		if in.Mode == "overwrite" {
			reason = "Overwrite requires a verified work-copy overwrite route and full-definition source revision."
		}
		return out, lifecycleFailure(op, in.WorkCopyLUID, reason, errs.PhaseSetup, errs.OutcomeNotAttempted, nil)
	}
	origin, err := j.Origin(ctx, in.Scope, in.WorkCopyLUID)
	if err != nil {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Could not read checkout provenance.", errs.PhaseSetup, errs.OutcomeNotAttempted, err)
	}
	out.SourceLUID = origin.SourceLUID
	if in.Mode == "overwrite" && (origin.SourceLUID == "" || origin.SourceRevision == "") {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Overwrite requires a verified source and its checkout revision.", errs.PhaseValidation, errs.OutcomeNotAttempted, nil)
	}
	draft := value.TDSAttempt{Operation: op, Scope: in.Scope, SourceLUID: origin.SourceLUID, SourceRevision: origin.SourceRevision, WorkCopyLUID: in.WorkCopyLUID, CompareDescription: in.Description != nil, CompleteCollection: !in.FromChanges && len(in.Fields) == 0}
	release, err := j.LockLifecycle(ctx, draft, in.AcknowledgeUnknown)
	if err != nil {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Save admission failed.", errs.PhaseSetup, errs.OutcomeNotAttempted, err)
	}
	defer release()
	if len(in.AcknowledgeUnknown) > 0 {
		out.Warnings = append(out.Warnings, "A previously submitted save may still take effect.")
	}
	out.Coverage = "exact fields"
	if in.FromChanges {
		out.Coverage = "exact changes file properties"
	} else if draft.CompleteCollection {
		out.Coverage = "complete bounded field-description collection"
	}
	dest, err := planTDSSave(ctx, p, in, &draft, &out)
	if err != nil {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Save preflight failed.", errs.PhaseValidation, errs.OutcomeNotAttempted, err)
	}
	token := draft.IntentDigest
	if in.ExpectPreview != "" && in.ExpectPreview != token {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "The requested save differs from its preview.", errs.PhaseValidation, errs.OutcomeNotAttempted, nil)
	}
	if preview {
		out.Status = "preview"
		out.PreviewToken = token
		return out, nil
	}
	out.Mode = "execute"
	dest, err = planTDSSave(ctx, p, in, &draft, &out)
	if err != nil || token != draft.IntentDigest {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Save preconditions changed before submission.", errs.PhaseValidation, errs.OutcomeNotAttempted, err)
	}
	a, err := j.Begin(ctx, draft, in.AcknowledgeUnknown)
	if err != nil {
		out.AttemptID = a.ID
		out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "work-copy", "--attempts", "--work-copy", in.WorkCopyLUID)}
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Could not retain the pending save.", errs.PhasePersistence, errs.OutcomeNotAttempted, err)
	}
	out.AttemptID = a.ID
	out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "work-copy", "--attempt", a.ID, "--verify")}
	if err := errors.Join(ctx.Err(), emitTDSAttempt(in.AttemptOutput, a.ID)); err != nil {
		return out, abortUndispatched(ctx, j, a, err)
	}
	receipt, submitErr := p.SubmitTDSSave(ctx, in.Mode, in.WorkCopyLUID, dest, in.Description)
	a.Status = receiptStatus(receipt, submitErr)
	a.RequestID = receipt.TableauRequestID
	id := receipt.DatasourceLUID
	if a.Status == "acknowledged" && (!value.ValidTDSLUID(id) || receipt.WorkCopyLUID != "" || strings.EqualFold(id, in.WorkCopyLUID) || (in.Mode == "overwrite" && !strings.EqualFold(id, a.SourceLUID)) || (in.Mode == "as-new" && strings.EqualFold(id, a.SourceLUID))) {
		a.Status = "unknown"
		submitErr = errors.New("the returned identity contradicts the selected save mode")
	}
	if a.Status == "acknowledged" {
		a.PublishedLUID = id
		a.Verification = "pending"
	} else {
		a.ReportedLUID = id
		a.UnexpectedIdentity = receipt.WorkCopyLUID
		out.UnexpectedIdentity = a.UnexpectedIdentity
	}
	out.WorkCopyAfterSave = "unknown"
	out.Status, out.PublishedLUID, out.ReportedLUID, out.RequestID = a.Status, a.PublishedLUID, a.ReportedLUID, a.RequestID
	if err := recordTDSReceipt(ctx, j, a); err != nil {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Could not persist the submit outcome.", errs.PhasePersistence, submitOutcome(a.Status), errors.Join(submitErr, err))
	}
	if a.Status != "acknowledged" || submitErr != nil {
		return out, lifecycleFailure(op, in.WorkCopyLUID, "Tableau did not confirm the requested published identity.", errs.PhaseSubmission, submitOutcome(a.Status), submitErr)
	}
	verified, verifyErr := verifySaveAttempt(ctx, p, p, a, in.Redact)
	out.Verification, out.Observations = verified.Verification, verified.Observations
	out.PublishedName, out.PublishedProjectLUID, out.DestinationMatch = verified.PublishedName, verified.PublishedProjectLUID, verified.DestinationMatch
	a.Verification = out.Verification
	if err := recordTDSReceipt(ctx, j, a); err != nil {
		return out, lifecycleFailure(op, a.PublishedLUID, "Could not retain the verification result.", errs.PhasePersistence, errs.OutcomeConfirmed, errors.Join(verifyErr, err))
	}
	return out, verifyErr
}

func planTDSSave(ctx context.Context, p TDSSaveProvider, in SaveWorkCopyInput, a *value.TDSAttempt, out *LifecycleOutput) (TDSDestination, error) {
	dest, err := p.TDSSaveDestination(ctx, in.Mode, in.WorkCopyLUID, a.SourceLUID, in.Name)
	if err != nil {
		return dest, err
	}
	if err := checkLifecycleMetadata(in.Redact, dest.Name, dest.ProjectLUID, dest.SourceRevision); err != nil {
		return TDSDestination{}, err
	}
	if dest.Name == "" || dest.ProjectLUID == "" {
		return dest, errors.New("the destination name and exact project must be established")
	}
	out.Name, out.ProjectLUID = dest.Name, dest.ProjectLUID
	if in.Mode == "as-new" {
		if dest.Name != in.Name {
			return dest, errors.New("the planned name differs from the requested name")
		}
		matches, err := p.FindDatasources(ctx, dest.Name, dest.ProjectLUID)
		if err != nil {
			return dest, err
		}
		if len(matches) > 0 {
			return dest, errors.New("the exact destination name is occupied")
		}
	} else if dest.SourceRevision == "" || dest.SourceRevision != a.SourceRevision {
		return dest, errors.New("the original published revision has changed or is unknown")
	}
	var selectors []EditSelector
	for _, ch := range in.Changes {
		selectors = append(selectors, ch.Selector())
	}
	for _, f := range in.Fields {
		selectors = append(selectors, EditSelector{ElementType: "FieldDescription", Name: f, Property: "description"})
	}
	observations, err := readSaveScope(ctx, p, "workcopy", in.WorkCopyLUID, selectors, a.CompleteCollection)
	out.Observations = observations
	if err != nil {
		return dest, err
	}
	a.Projections = nil
	for i, o := range observations {
		if in.FromChanges {
			want := in.Changes[i].Intended()
			out.Observations[i].Expected = &want
			if o.State != want {
				out.Observations[i].Status = "mismatch"
				return dest, errors.New("the current work copy differs from the changes file")
			}
		}
		a.Projections = append(a.Projections, value.TDSProjection{ElementType: o.Selector.ElementType, Name: o.Selector.Name, Property: o.Selector.Property, Digest: lifecycleDigest(o.State)})
	}
	a.DestinationDigest = tdsDestinationDigest(dest.Name, dest.ProjectLUID, in.Description)
	a.IntentDigest = lifecycleDigest([]any{a.Scope, a.Operation, a.SourceLUID, a.SourceRevision, a.WorkCopyLUID, a.DestinationDigest, a.CompleteCollection, a.Projections, in.FromChanges, in.Changes})
	return dest, nil
}

func tdsDestinationDigest(name, project string, description *string) string {
	return lifecycleDigest([]any{name, project, description})
}

func readSaveScope(ctx context.Context, p TDSReader, kind, id string, selectors []EditSelector, collection bool) ([]SaveObservation, error) {
	var obs []SaveObservation
	if collection {
		read, err := p.ReadTDSElements(ctx, kind, id, "FieldDescription", "")
		if err != nil {
			return nil, err
		}
		if read.Absent || len(read.Elements) > 100 {
			return nil, errors.New("field-description collection is incomplete or exceeds 100 entries; select exact fields")
		}
		seen := map[string]bool{}
		for _, e := range read.Elements {
			if e.Error != nil || e.Description == nil || e.Name == "" || seen[e.Name] {
				return obs, errors.New("field-description collection contains incomplete or duplicate records")
			}
			seen[e.Name] = true
			obs = append(obs, SaveObservation{Selector: EditSelector{ElementType: "FieldDescription", Name: e.Name, Property: "description"}, State: EditState{Presence: "present", Value: *e.Description}, Status: "observed"})
		}
		slices.SortFunc(obs, func(a, b SaveObservation) int { return cmp.Compare(a.Selector.Name, b.Selector.Name) })
		return obs, nil
	}
	for _, s := range selectors {
		state, err := ObserveEdit(ctx, p, kind, id, s)
		o := SaveObservation{Selector: s, State: state, Status: "observed"}
		if err != nil || state.Presence == "unknown" {
			o.Status = "unknown"
			return append(obs, o), errors.Join(errors.New("the selected property could not be verified"), err)
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// VerifySaveAttempt observes the current published target even after a process
// interruption. Matching values cannot establish attribution to a lost submit.
func VerifySaveAttempt(ctx context.Context, r Resolver, p TDSReader, a value.TDSAttempt) (LifecycleOutput, error) {
	var redact func(string) string
	if provider, ok := p.(interface{ TDSRedactor() func(string) string }); ok {
		redact = provider.TDSRedactor()
	}
	return verifySaveAttempt(ctx, r, p, a, redact)
}

func verifySaveAttempt(ctx context.Context, r Resolver, p TDSReader, a value.TDSAttempt, redact func(string) string) (LifecycleOutput, error) {
	op := a.Operation
	out := LifecycleOutput{Operation: op, Mode: "verify", Status: a.Status, AttemptID: a.ID, WorkCopyLUID: a.WorkCopyLUID, SourceLUID: a.SourceLUID, PublishedLUID: a.PublishedLUID, ReportedLUID: a.ReportedLUID, UnexpectedIdentity: a.UnexpectedIdentity, RequestID: a.RequestID, Verification: "unknown"}
	id := a.PublishedLUID
	if id == "" && op == "datasource.save.overwrite" {
		id = a.SourceLUID
	}
	fail := func(msg string, err error) (LifecycleOutput, error) {
		return out, lifecycleFailure(op, id, msg, errs.PhaseVerification, submitOutcome(a.Status), err)
	}
	if id == "" || a.DestinationDigest == "" {
		return fail("No attributable published identity and retained comparison scope are available.", nil)
	}
	actual, err := r.ResolveDatasource(ctx, identity.Selector{LUID: identity.LUID(id)})
	if err != nil || !strings.EqualFold(id, actual.LUID) {
		return fail("Could not verify the exact published identity.", err)
	}
	if err := checkLifecycleMetadata(redact, actual.LUID, actual.Name, actual.ProjectLUID, actual.Description); err != nil {
		return fail("Published destination metadata contains protected credential material.", nil)
	}
	out.PublishedName, out.PublishedProjectLUID = actual.Name, actual.ProjectLUID
	var desc *string
	if a.CompareDescription {
		if !actual.DescriptionPresent && actual.Description == "" {
			return fail("Published datasource readback omitted the requested description.", nil)
		}
		desc = &actual.Description
	}
	metadataMatch := tdsDestinationDigest(actual.Name, actual.ProjectLUID, desc) == a.DestinationDigest
	out.DestinationMatch = &metadataMatch
	sels := make([]EditSelector, 0, len(a.Projections))
	for _, v := range a.Projections {
		sels = append(sels, EditSelector{v.ElementType, v.Name, v.Property})
	}
	observed, err := readSaveScope(ctx, p, "datasource", id, sels, a.CompleteCollection)
	out.Observations = observed
	if err != nil {
		return fail("Published property verification is incomplete.", err)
	}
	want := map[EditSelector]string{}
	for i, s := range sels {
		want[s] = a.Projections[i].Digest
	}
	match := len(observed) == len(want)
	for i, o := range observed {
		if want[o.Selector] != lifecycleDigest(o.State) {
			out.Observations[i].Status = "mismatch"
			match = false
		} else {
			out.Observations[i].Status = "matched"
		}
		delete(want, o.Selector)
	}
	for _, s := range sels {
		if _, missing := want[s]; missing {
			out.Observations = append(out.Observations, SaveObservation{Selector: s, State: EditState{Presence: "absent"}, Status: "mismatch"})
		}
	}
	if !metadataMatch || !match {
		out.Verification = "mismatch"
		return fail(fmt.Sprintf("Published comparison differs (destination matches: %t, selected properties match: %t).", metadataMatch, match), nil)
	}
	out.Verification = "matched"
	if a.Status != "acknowledged" {
		return fail("The current published values match, but the submission outcome is unresolved.", nil)
	}
	return out, nil
}
