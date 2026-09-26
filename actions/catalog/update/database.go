package update

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type DatabaseInput struct {
	Environment, Site, ID string
	TargetResolved        bool
	Description           *string
	ContactLUID           *string
	AddTags, RemoveTags   []string
}
type DatabaseReader interface {
	GetDatabase(context.Context, string) (value.MetadataDatabase, error)
}
type DatabaseWriter interface {
	UpdateDatabase(context.Context, string, value.MetadataUpdate) (value.MetadataDatabase, error)
	AddDatabaseTags(context.Context, string, []string) ([]string, error)
	DeleteDatabaseTag(context.Context, string, string) error
}
type DatabaseAction struct {
	reader DatabaseReader
	writer DatabaseWriter
}

func NewDatabase(r DatabaseReader, w DatabaseWriter) *DatabaseAction {
	return &DatabaseAction{reader: r, writer: w}
}

type DatabasePlan struct {
	Mode        string                 `json:"mode"`
	Operation   string                 `json:"operation"`
	Environment string                 `json:"environment"`
	Site        string                 `json:"site"`
	Target      value.MetadataIdentity `json:"target"`
	Changes     []Change               `json:"changes"`
	NoOp        bool                   `json:"no_op"`
}
type DatabaseResult struct {
	Status       string                 `json:"status"`
	Identity     value.MetadataIdentity `json:"identity"`
	Description  *string                `json:"description,omitempty"`
	ContactLUID  string                 `json:"contact_luid,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
	TagsObserved bool                   `json:"tags_observed"`
	Completed    []string               `json:"completed"`
	Failed       string                 `json:"failed,omitempty"`
}
type DatabaseOutput struct {
	Plan   DatabasePlan    `json:"plan"`
	Result *DatabaseResult `json:"result,omitempty"`
	Help   []string        `json:"help,omitempty"`
}

func (o DatabaseOutput) CompactOutput() any { return o }
func (o DatabaseOutput) FullOutput() any    { return o }
func ValidateDatabaseInput(in DatabaseInput) error {
	for _, id := range []string{in.ID} {
		if id != "" && (id != strings.TrimSpace(id) || strings.ContainsAny(id, "\x00\r\n")) {
			return databaseUsage("identities must be exact and contain no surrounding whitespace or control characters")
		}
	}
	if strings.TrimSpace(in.ID) == "" {
		return databaseUsage("an exact REST --id is required")
	}

	if in.Description == nil && in.ContactLUID == nil && len(in.AddTags) == 0 && len(in.RemoveTags) == 0 {
		return databaseUsage("supply a description, supported contact, or tag change")
	}
	if in.Description != nil && (strings.TrimSpace(*in.Description) == "" || len(*in.Description) > 65536) {
		return databaseUsage("description must be nonempty and bounded; clearing is not verified")
	}
	if in.ContactLUID != nil && strings.TrimSpace(*in.ContactLUID) == "" {
		return databaseUsage("contact clearing is not verified; supply an exact contact LUID")
	}
	if reason := validateTags(in.AddTags, in.RemoveTags); reason != "" {
		return databaseUsage(reason)
	}
	return nil
}

func (a *DatabaseAction) Execute(ctx context.Context, in DatabaseInput, preview bool) (DatabaseOutput, error) {
	if err := ValidateDatabaseInput(in); err != nil {
		return DatabaseOutput{}, err
	}
	if strings.TrimSpace(in.Environment) == "" || (!in.TargetResolved && strings.TrimSpace(in.Site) == "") {
		return DatabaseOutput{}, databaseUsage("resolve an explicit mutation environment")
	}
	if a == nil || a.reader == nil || a.writer == nil {
		return DatabaseOutput{}, databaseUsage("catalog database update is not configured")
	}
	target, err := a.reader.GetDatabase(ctx, in.ID)
	if err != nil {
		return DatabaseOutput{}, databaseFailure(in, "read", nil, errs.OutcomeNotAttempted, err)
	}
	if err = databaseValidTarget(target, in); err != nil {
		return DatabaseOutput{}, databaseFailure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove := databaseChanged(target, in)
	out := DatabaseOutput{Plan: DatabasePlan{Mode: "preview", Operation: "catalog.database.update", Environment: in.Environment, Site: in.Site, Target: target.MetadataIdentity, Changes: changes, NoOp: len(changes) == 0}}
	if preview {
		return out, nil
	}
	current, err := a.reader.GetDatabase(ctx, in.ID)
	if err != nil {
		return out, databaseFailure(in, "revalidate", nil, errs.OutcomeNotAttempted, err)
	}
	if err = databaseValidTarget(current, in); err != nil {
		return out, databaseFailure(in, "identity", nil, errs.OutcomeNotAttempted, err)
	}
	request, changes, add, remove = databaseChanged(current, in)
	out.Plan.Mode = "execute"
	out.Plan.Target = current.MetadataIdentity
	out.Plan.Changes = changes
	out.Plan.NoOp = len(changes) == 0
	out.Result = &DatabaseResult{Status: "updated", Identity: current.MetadataIdentity, Description: current.Description, ContactLUID: current.ContactLUID, Tags: slices.Clone(current.Tags), TagsObserved: current.TagsObserved, Completed: []string{}}
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
				out.Result.Identity = retainedIdentity(current.MetadataIdentity, updated.MetadataIdentity)
				out.Result.Description = updated.Description
				out.Result.ContactLUID = updated.ContactLUID
				out.Result.Tags = slices.Clone(updated.Tags)
				out.Result.TagsObserved = updated.TagsObserved
				outcome = errs.OutcomeConfirmed
			}
			return out, databaseFailure(in, "properties", out.Result.Completed, outcome, e)
		}
		if updated.LUID != in.ID {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, databaseFailure(in, "properties", out.Result.Completed, errs.OutcomeUnknown, fmt.Errorf("updated identity does not match target"))
		}
		out.Result.Identity = retainedIdentity(current.MetadataIdentity, updated.MetadataIdentity)
		out.Result.Description = updated.Description
		out.Result.ContactLUID = updated.ContactLUID
		out.Result.Tags = slices.Clone(updated.Tags)
		out.Result.TagsObserved = updated.TagsObserved
		if request.Description != nil && (updated.Description == nil || *updated.Description != *request.Description) {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, databaseFailure(in, "properties", out.Result.Completed, errs.OutcomeConfirmed, propertyVerification("description"))
		}
		if request.ContactLUID != nil && updated.ContactLUID != *request.ContactLUID {
			out.Result.Status = "partial"
			out.Result.Failed = "properties"
			return out, databaseFailure(in, "properties", out.Result.Completed, errs.OutcomeConfirmed, propertyVerification("contact"))
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
			e = verifyTagAcknowledgment(add, acknowledged)
		}
		if e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "add_tags"
			return out, databaseFailure(in, "add_tags", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "add_tags")
		out.Result.Tags = slices.Clone(acknowledged)
		out.Result.TagsObserved = true
	}
	for _, tag := range remove {
		if e := a.writer.DeleteDatabaseTag(ctx, in.ID, tag); e != nil {
			out.Result.Status = "partial"
			out.Result.Failed = "remove_tag:" + tag
			return out, databaseFailure(in, "remove_tag", out.Result.Completed, errs.OutcomeUnknown, e)
		}
		out.Result.Completed = append(out.Result.Completed, "remove_tag:"+tag)
		out.Result.Tags = nil
		out.Result.TagsObserved = false
	}
	return out, nil
}
func databaseValidTarget(v value.MetadataDatabase, in DatabaseInput) error {
	if v.LUID != in.ID {
		return fmt.Errorf("returned database has a different REST identity")
	}
	return nil
}
func databaseChanged(v value.MetadataDatabase, in DatabaseInput) (value.MetadataUpdate, []Change, []string, []string) {
	patch := value.MetadataUpdate{}
	changes := []Change{}
	if in.Description != nil && (v.Description == nil || *v.Description != *in.Description) {
		patch.Description = new(*in.Description)
		changes = append(changes, Change{Property: "description", Before: v.Description, After: *in.Description})
	}
	if in.ContactLUID != nil && v.ContactLUID != *in.ContactLUID {
		patch.ContactLUID = new(*in.ContactLUID)
		changes = append(changes, Change{Property: "contact", Before: new(v.ContactLUID), After: *in.ContactLUID})
	}
	tags, add, remove := tagChanges(v.Tags, v.TagsObserved, in.AddTags, in.RemoveTags)
	changes = append(changes, tags...)
	return patch, changes, add, remove
}
func databaseUsage(s string) error {
	return &errs.Error{ID: "catalog.database.update.usage", Kind: errs.KindUsage, Operation: "catalog.database.update", Summary: s, Retryable: errs.Bool(false), CorrectiveAction: "Correct the requested metadata update and preview it."}
}
func databaseFailure(in DatabaseInput, step string, completed []string, outcome errs.Outcome, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Inspect the exact asset before retrying: "+commandhint.Environment(in.Environment, "catalog", "database", "inspect", "--id", in.ID))
	return &errs.Error{ID: "catalog.database.update." + step, Kind: errs.KindOperation, Operation: "catalog.database.update", Environment: in.Environment, Site: in.Site, Resource: in.ID, Summary: "Catalog database metadata update failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause), Completed: completed, Failed: step, Phase: failurePhase(cause, outcome), Outcome: outcome}
}
