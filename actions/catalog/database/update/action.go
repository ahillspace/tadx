package update

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"slices"
	"strings"
	"unicode/utf8"
)

type Input struct {
	Environment, Site, ID string
	TargetResolved        bool
	Description           *string
	ContactLUID           *string
	AddTags, RemoveTags   []string
}
type Reader interface {
	GetDatabase(context.Context, string) (value.MetadataDatabase, error)
}
type Writer interface {
	UpdateDatabase(context.Context, string, value.MetadataUpdate) (value.MetadataDatabase, error)
	AddDatabaseTags(context.Context, string, []string) ([]string, error)
	DeleteDatabaseTag(context.Context, string, string) error
}
type Action struct {
	reader Reader
	writer Writer
}

func New(r Reader, w Writer) *Action { return &Action{reader: r, writer: w} }

type Change struct {
	Property string  `json:"property"`
	Before   *string `json:"before"`
	After    string  `json:"after"`
}
type Plan struct {
	Mode        string                 `json:"mode"`
	Operation   string                 `json:"operation"`
	Environment string                 `json:"environment"`
	Site        string                 `json:"site"`
	Target      value.MetadataIdentity `json:"target"`
	Changes     []Change               `json:"changes"`
	NoOp        bool                   `json:"no_op"`
}
type Result struct {
	Status    string                 `json:"status"`
	Identity  value.MetadataIdentity `json:"identity"`
	Completed []string               `json:"completed"`
	Failed    string                 `json:"failed,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help,omitempty"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }
func ValidateInput(in Input) error {
	for _, id := range []string{in.ID} {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return usage("identities must be exact and contain no surrounding whitespace or control characters")
		}
	}
	if strings.TrimSpace(in.ID) == "" {
		return usage("an exact REST --id is required")
	}

	if in.Description == nil && in.ContactLUID == nil && len(in.AddTags) == 0 && len(in.RemoveTags) == 0 {
		return usage("supply a description, supported contact, or tag change")
	}
	if in.Description != nil && (strings.TrimSpace(*in.Description) == "" || len(*in.Description) > 65536) {
		return usage("description must be nonempty and bounded; clearing is not verified")
	}
	if in.ContactLUID != nil && strings.TrimSpace(*in.ContactLUID) == "" {
		return usage("contact clearing is not verified; supply an exact contact LUID")
	}
	if len(in.AddTags)+len(in.RemoveTags) > 100 {
		return usage("at most 100 tag changes are allowed per item")
	}
	for _, tag := range in.AddTags {
		if strings.TrimSpace(tag) == "" || utf8.RuneCountInString(tag) > 128 {
			return usage("added tags must contain 1 to 128 characters")
		}
	}
	for _, tag := range in.RemoveTags {
		if strings.TrimSpace(tag) == "" || tag != strings.TrimSpace(tag) || strings.ContainsAny(tag, "\x00\r\n") {
			return usage("removed tags must be exact nonempty selectors without surrounding whitespace or control characters")
		}
	}
	for _, tag := range in.AddTags {
		if slices.Contains(in.RemoveTags, tag) {
			return usage("a tag cannot be both added and removed")
		}
	}
	return nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if strings.TrimSpace(in.Environment) == "" || (!in.TargetResolved && strings.TrimSpace(in.Site) == "") {
		return Output{}, usage("resolve an explicit mutation environment")
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return Output{}, usage("catalog database update is not configured")
	}
	target, err := a.reader.GetDatabase(ctx, in.ID)
	if err != nil {
		return Output{}, failure(in, "read", nil, errs.OutcomeNotAttempted, err)
	}
	if err = validTarget(target, in); err != nil {
		return Output{}, failure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove := changed(target, in)
	out := Output{Plan: Plan{Mode: "preview", Operation: "catalog.database.update", Environment: in.Environment, Site: in.Site, Target: target.MetadataIdentity, Changes: changes, NoOp: len(changes) == 0}}
	if preview {
		return out, nil
	}
	current, err := a.reader.GetDatabase(ctx, in.ID)
	if err != nil {
		return out, failure(in, "revalidate", nil, errs.OutcomeNotAttempted, err)
	}
	if err = validTarget(current, in); err != nil {
		return out, failure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove = changed(current, in)
	out.Plan.Mode = "execute"
	out.Plan.Target = current.MetadataIdentity
	out.Plan.Changes = changes
	out.Plan.NoOp = len(changes) == 0
	out.Result = &Result{Status: "updated", Identity: current.MetadataIdentity, Completed: []string{}}
	out.Help = []string{commandhint.Environment(in.Environment, "catalog", "database", "inspect", "--id", in.ID)}
	if out.Plan.NoOp {
		out.Result.Status = "unchanged"
		return out, nil
	}
	if request.Description != nil || request.ContactLUID != nil {
		updated, e := a.writer.UpdateDatabase(ctx, in.ID, request)
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			outcome := errs.OutcomeUnknown
			if updated.LUID == in.ID {
				out.Result.Identity = updated.MetadataIdentity
				outcome = errs.OutcomeConfirmed
			}
			return out, failure(in, "properties", out.Result.Completed, outcome, e)
		}
		if updated.LUID != in.ID {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, failure(in, "properties", out.Result.Completed, errs.OutcomeUnknown, fmt.Errorf("updated identity does not match target"))
		}
		if request.Description != nil {
			out.Result.Completed = append(out.Result.Completed, "description")
		}
		if request.ContactLUID != nil {
			out.Result.Completed = append(out.Result.Completed, "contact")
		}
	}
	if len(add) > 0 {
		acknowledged, e := a.writer.AddDatabaseTags(ctx, in.ID, add)
		if e == nil {
			for _, tag := range add {
				if !slices.Contains(acknowledged, tag) {
					e = &errs.Error{Kind: errs.KindOperation, Summary: "Tag response did not confirm every requested addition.", Phase: errs.PhaseVerification, Retryable: errs.Bool(false)}
					break
				}
			}
		}
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "add_tags"
			return out, failure(in, "add_tags", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "add_tags")
	}
	for _, tag := range remove {
		if e := a.writer.DeleteDatabaseTag(ctx, in.ID, tag); e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "remove_tag:" + tag
			return out, failure(in, "remove_tag", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "remove_tag:"+tag)
	}
	return out, nil
}
func validTarget(v value.MetadataDatabase, in Input) error {
	if v.LUID != in.ID {
		return fmt.Errorf("returned database has a different REST identity")
	}
	return nil
}
func changed(v value.MetadataDatabase, in Input) (value.MetadataUpdate, []Change, []string, []string) {
	patch := value.MetadataUpdate{}
	changes := []Change{}
	add := []string{}
	remove := []string{}
	if in.Description != nil && (v.Description == nil || *v.Description != *in.Description) {
		s := *in.Description
		patch.Description = &s
		changes = append(changes, Change{Property: "description", Before: v.Description, After: s})
	}
	if in.ContactLUID != nil && v.ContactLUID != *in.ContactLUID {
		s := *in.ContactLUID
		before := v.ContactLUID
		patch.ContactLUID = &s
		changes = append(changes, Change{Property: "contact", Before: &before, After: s})
	}
	for _, tag := range in.AddTags {
		if !slices.Contains(add, tag) && (!v.TagsObserved || !slices.Contains(v.Tags, tag)) {
			add = append(add, tag)
			changes = append(changes, Change{Property: "add_tag", After: tag})
		}
	}
	for _, tag := range in.RemoveTags {
		if !slices.Contains(remove, tag) && (!v.TagsObserved || slices.Contains(v.Tags, tag)) {
			remove = append(remove, tag)
			changes = append(changes, Change{Property: "remove_tag", After: tag})
		}
	}
	return patch, changes, add, remove
}
func usage(s string) error {
	return &errs.Error{ID: "catalog.database.update.usage", Kind: errs.KindUsage, Operation: "catalog.database.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the requested metadata update and preview it."}
}
func failure(in Input, step string, completed []string, outcome errs.Outcome, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Inspect the exact asset before retrying: "+commandhint.Environment(in.Environment, "catalog", "database", "inspect", "--id", in.ID))
	phase := errs.PhaseSubmission
	var structured *errs.Error
	var verification interface{ VerificationFailed() bool }
	if (errors.As(cause, &structured) && structured.Phase == errs.PhaseVerification) || (errors.As(cause, &verification) && verification.VerificationFailed()) {
		phase = errs.PhaseVerification
	}
	if outcome == errs.OutcomeNotAttempted {
		phase = errs.PhaseValidation
	}
	return &errs.Error{ID: "catalog.database.update." + step, Kind: errs.KindOperation, Operation: "catalog.database.update", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog database metadata update failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause), Completed: completed, Failed: step, Phase: phase, Outcome: outcome}
}
