package group

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type DeleteInput struct {
	Environment, Site, GroupLUID string
}
type DeleteGroup struct {
	LUID   string `json:"luid"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
}
type DeletePlan struct {
	Mode             string      `json:"mode"`
	Operation        string      `json:"operation"`
	Environment      string      `json:"environment"`
	Site             string      `json:"site"`
	Target           DeleteGroup `json:"target"`
	DeletesUsers     bool        `json:"deletes_users"`
	PermissionImpact string      `json:"permission_impact"`
}
type DeleteResult struct {
	Status           string `json:"status"`
	GroupLUID        string `json:"group_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type DeleteOutput struct {
	Plan   DeletePlan    `json:"plan"`
	Result *DeleteResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type DeleteCompactMutationResult struct {
	Status    string `json:"status"`
	GroupLUID string `json:"group_luid"`
}
type DeleteCompactResult struct {
	Plan    DeletePlan                   `json:"plan"`
	Result  *DeleteCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactMutationResult
	if o.Result != nil {
		result = &DeleteCompactMutationResult{Status: o.Result.Status, GroupLUID: o.Result.GroupLUID}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o DeleteOutput) FullOutput() any {
	return o
}

type DeleteWriter interface {
	DeleteGroup(context.Context, string) (DeleteResult, error)
}

func Delete(ctx context.Context, resolver Resolver, deleter DeleteWriter, in DeleteInput, preview bool) (DeleteOutput, error) {
	record, err := resolver.ResolveGroup(ctx, Selector{LUID: in.GroupLUID}, false)
	if err != nil {
		return DeleteOutput{}, err
	}
	g := deleteGroup(record)
	out := DeleteOutput{Plan: DeletePlan{Mode: "preview", Operation: "admin.group.delete", Environment: in.Environment, Site: in.Site, Target: g, DeletesUsers: false, PermissionImpact: "unknown"}, Help: []string{"Run without --preview to delete this exact group without deleting users."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveGroup(ctx, Selector{LUID: in.GroupLUID}, false)
	if err != nil {
		return DeleteOutput{}, err
	}
	if !reflect.DeepEqual(deleteGroup(current), g) {
		return DeleteOutput{}, errors.New("the group delete target changed during revalidation")
	}
	result, err := deleter.DeleteGroup(ctx, g.LUID)
	if err != nil {
		if result.Status == "unknown" {
			luid := result.GroupLUID
			if luid == "" {
				luid = g.LUID
			}
			return DeleteOutput{}, &errs.Error{ID: "admin.group.delete.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.delete", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The group delete outcome could not be determined safely.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact group and Tableau request before retrying: " + commandhint.Environment(in.Environment, "admin", "group", "inspect", "--id", luid), TableauRequestID: result.TableauRequestID}
		}
		return DeleteOutput{}, err
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "group", "list")}
	return out, nil
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateDeleteInput(in DeleteInput) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return &errs.Error{ID: "admin.group.delete.usage", Kind: errs.KindUsage, Operation: "admin.group.delete", Summary: "admin group delete requires explicit environment and group LUID", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and group LUID.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group delete requires explicit environment and group LUID"}}}
	}
	return nil
}

func deleteGroup(v Record) DeleteGroup {
	return DeleteGroup{LUID: v.LUID, Name: v.Name, Domain: v.Domain}
}
