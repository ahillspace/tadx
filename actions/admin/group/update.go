package group

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

const maxDesiredMembers = 1000

type UpdateInput struct {
	Environment, Site, GroupLUID string
	Name, MinimumSiteRole        *string
	ExternalUserEnabled          *bool
	MembershipSet                bool
	DesiredMemberLUIDs           []string
}
type UpdateMember struct {
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
}
type UpdateGroup struct {
	LUID                string         `json:"luid"`
	Name                string         `json:"name"`
	Domain              string         `json:"domain,omitempty"`
	MinimumSiteRole     string         `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool          `json:"external_user_enabled,omitempty"`
	Members             []UpdateMember `json:"members,omitempty"`
	RequestID           string         `json:"-"`
	MutationStatus      string         `json:"-"`
}
type UpdateRequest = value.AdminUpdateGroupRequest
type UpdateChange struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type MembershipDiff struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}
type UpdatePlan struct {
	Mode        string          `json:"mode"`
	Operation   string          `json:"operation"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Target      UpdateGroup     `json:"target"`
	Requested   UpdateRequest   `json:"requested"`
	Changes     []UpdateChange  `json:"changes"`
	Membership  *MembershipDiff `json:"membership,omitempty"`
	NoOp        bool            `json:"no_op"`
}
type UpdateResult struct {
	Status                   string          `json:"status"`
	GroupLUID                string          `json:"group_luid"`
	Group                    *ConfirmedGroup `json:"group,omitempty"`
	AddedUserLUIDs           []string        `json:"added_user_luids,omitempty"`
	RemovedUserLUIDs         []string        `json:"removed_user_luids,omitempty"`
	Added                    int             `json:"added"`
	Removed                  int             `json:"removed"`
	TableauRequestIDs        []string        `json:"tableau_request_ids,omitempty"`
	TableauRequestIDsOmitted int             `json:"tableau_request_ids_omitted,omitempty"`
}

// ConfirmedGroup contains only attributes returned by the metadata mutation.
// Direct membership inventory is deliberately excluded from the receipt.
type ConfirmedGroup struct {
	LUID                string `json:"luid"`
	Name                string `json:"name,omitempty"`
	Domain              string `json:"domain,omitempty"`
	MinimumSiteRole     string `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool  `json:"external_user_enabled,omitempty"`
}
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type CompactPlan struct {
	Mode        string         `json:"mode"`
	Operation   string         `json:"operation"`
	Environment string         `json:"environment"`
	Site        string         `json:"site"`
	GroupLUID   string         `json:"group_luid"`
	Requested   *UpdateRequest `json:"requested,omitempty"`
	Changes     []UpdateChange `json:"changes,omitempty"`
	ChangeCount int            `json:"change_count"`
	AddCount    int            `json:"add_count"`
	RemoveCount int            `json:"remove_count"`
	NoOp        bool           `json:"no_op"`
}
type UpdateCompactResult struct {
	Plan    CompactPlan                  `json:"plan"`
	Result  *UpdateCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}
type UpdateCompactMutationResult struct {
	Status                  string          `json:"status"`
	GroupLUID               string          `json:"group_luid"`
	Added                   int             `json:"added"`
	Removed                 int             `json:"removed"`
	Group                   *ConfirmedGroup `json:"group,omitempty"`
	AddedUserLUIDs          []string        `json:"added_user_luids,omitempty"`
	RemovedUserLUIDs        []string        `json:"removed_user_luids,omitempty"`
	AddedUserLUIDsOmitted   int             `json:"added_user_luids_omitted,omitempty"`
	RemovedUserLUIDsOmitted int             `json:"removed_user_luids_omitted,omitempty"`
}

func (o UpdateOutput) CompactOutput() any {
	add, remove := 0, 0
	if o.Plan.Membership != nil {
		add, remove = len(o.Plan.Membership.Add), len(o.Plan.Membership.Remove)
	}
	var result *UpdateCompactMutationResult
	if o.Result != nil {
		result = &UpdateCompactMutationResult{Status: o.Result.Status, GroupLUID: o.Result.GroupLUID, Added: o.Result.Added, Removed: o.Result.Removed}
		result.Group = o.Result.Group
		result.AddedUserLUIDs, result.AddedUserLUIDsOmitted = boundedMemberIDs(o.Result.AddedUserLUIDs)
		result.RemovedUserLUIDs, result.RemovedUserLUIDsOmitted = boundedMemberIDs(o.Result.RemovedUserLUIDs)
	}
	plan := CompactPlan{Mode: o.Plan.Mode, Operation: o.Plan.Operation, Environment: o.Plan.Environment, Site: o.Plan.Site, GroupLUID: o.Plan.Target.LUID, ChangeCount: len(o.Plan.Changes), AddCount: add, RemoveCount: remove, NoOp: o.Plan.NoOp}
	if o.Plan.Mode == "preview" {
		plan.Requested = new(o.Plan.Requested)
		plan.Changes = o.Plan.Changes
	}
	return UpdateCompactResult{Plan: plan, Result: result, Details: "--full", Help: o.Help}
}
func boundedMemberIDs(ids []string) ([]string, int) {
	const limit = 10
	if len(ids) > limit {
		return append([]string(nil), ids[:limit]...), len(ids) - limit
	}
	return append([]string(nil), ids...), 0
}
func (o UpdateOutput) FullOutput() any {
	out := o
	if out.Result != nil {
		result := *out.Result
		result.TableauRequestIDs = append([]string(nil), result.TableauRequestIDs...)
		if len(result.TableauRequestIDs) > 2001 {
			result.TableauRequestIDsOmitted = len(result.TableauRequestIDs) - 2001
			result.TableauRequestIDs = result.TableauRequestIDs[:2001]
		}
		out.Result = &result
	}
	return out
}

type UpdateWriter interface {
	UpdateGroup(context.Context, string, UpdateRequest) (value.AdminGroup, error)
}
type MembershipWriter interface {
	AddGroupUser(context.Context, string, string) (string, error)
	RemoveGroupUser(context.Context, string, string) (string, error)
}

func Update(ctx context.Context, resolver Resolver, updater UpdateWriter, members MembershipWriter, in UpdateInput, preview bool) (UpdateOutput, error) {
	record, err := resolver.ResolveGroup(ctx, Selector{LUID: in.GroupLUID}, in.MembershipSet)
	if err != nil {
		return UpdateOutput{}, err
	}
	group := updateGroup(record)
	if in.MembershipSet && len(group.Members) > maxDesiredMembers {
		return UpdateOutput{}, errors.New("current group membership exceeds the 1000-member action bound")
	}
	request := UpdateRequest{Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled}
	changes := groupChanges(group, request)
	var membership *MembershipDiff
	if in.MembershipSet {
		diff := membershipDiff(group.Members, in.DesiredMemberLUIDs)
		membership = &diff
	}
	noOp := len(changes) == 0 && (membership == nil || (len(membership.Add) == 0 && len(membership.Remove) == 0))
	out := UpdateOutput{Plan: UpdatePlan{Mode: "preview", Operation: "admin.group.update", Environment: in.Environment, Site: in.Site, Target: group, Requested: request, Changes: changes, Membership: membership, NoOp: noOp}, Help: []string{"Run without --preview to update this exact group and converge direct membership."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveGroup(ctx, Selector{LUID: in.GroupLUID}, in.MembershipSet)
	if err != nil {
		return UpdateOutput{}, err
	}
	if !reflect.DeepEqual(updateGroup(current), group) {
		return UpdateOutput{}, errors.New("the group or its direct membership changed during revalidation")
	}
	result := &UpdateResult{Status: "unchanged", GroupLUID: group.LUID}
	out.Result = result
	if noOp {
		return out, nil
	}
	if len(changes) > 0 {
		updated, err := updater.UpdateGroup(ctx, group.LUID, request)
		if err != nil {
			if updated.MutationStatus == "unknown" {
				return UpdateOutput{}, updateOutcomeUnknown(in, group.LUID, updated.RequestID, err)
			}
			return UpdateOutput{}, err
		}
		if updated.RequestID != "" {
			result.TableauRequestIDs = append(result.TableauRequestIDs, updated.RequestID)
		}
		result.Group = &ConfirmedGroup{LUID: updated.LUID, Name: updated.Name, Domain: updated.Domain, MinimumSiteRole: updated.MinimumSiteRole, ExternalUserEnabled: updated.ExternalUserEnabled}
	}
	completed := make([]string, 0, 1+result.Added+result.Removed)
	if len(changes) > 0 {
		completed = append(completed, "group.metadata.update")
	}
	if membership != nil {
		for _, luid := range membership.Add {
			id, err := members.AddGroupUser(ctx, group.LUID, luid)
			if err != nil {
				result.Status = "partial"
				return out, partialError(in, group.LUID, completed, "member.add:"+luid, err)
			}
			result.Added++
			result.AddedUserLUIDs = append(result.AddedUserLUIDs, luid)
			completed = append(completed, "member.add:"+luid)
			if id != "" {
				result.TableauRequestIDs = append(result.TableauRequestIDs, id)
			}
		}
		for _, luid := range membership.Remove {
			id, err := members.RemoveGroupUser(ctx, group.LUID, luid)
			if err != nil {
				result.Status = "partial"
				return out, partialError(in, group.LUID, completed, "member.remove:"+luid, err)
			}
			result.Removed++
			result.RemovedUserLUIDs = append(result.RemovedUserLUIDs, luid)
			completed = append(completed, "member.remove:"+luid)
			if id != "" {
				result.TableauRequestIDs = append(result.TableauRequestIDs, id)
			}
		}
	}
	result.Status = "updated"
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "group", "inspect", "--id", group.LUID, "--members")}
	return out, nil
}

func updateUsage(field, message string) error {
	return &errs.Error{ID: "admin.group.update.usage", Kind: errs.KindUsage, Operation: "admin.group.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the group update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func updateOutcomeUnknown(in UpdateInput, luid, requestID string, cause error) error {
	return &errs.Error{ID: "admin.group.update.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.update", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The group update outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact group and Tableau request before retrying: " + commandhint.Environment(in.Environment, "admin", "group", "inspect", "--id", luid), TableauRequestID: requestID}
}
func partialError(input UpdateInput, groupLUID string, completed []string, failed string, cause error) error {
	return &errs.Error{ID: "admin.group.update.partial", Kind: errs.KindOperation, Operation: "admin.group.update", Resource: groupLUID, Environment: input.Environment, Site: input.Site, Summary: "Group update stopped after a partial remote mutation.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Re-read the exact group membership before reviewing a new update plan: " + commandhint.Environment(input.Environment, "admin", "group", "inspect", "--id", groupLUID, "--members"), Completed: append([]string(nil), completed...), Failed: failed, TableauRequestID: errs.TableauRequestID(cause)}
}
func normalizeDesired(values []string, enabled bool) ([]string, error) {
	if !enabled {
		return nil, nil
	}
	if len(values) > maxDesiredMembers {
		return nil, updateUsage("members", "desired group membership exceeds the 1000-member action bound")
	}
	seen := map[string]bool{}
	result := append([]string(nil), values...)
	for _, v := range result {
		if v == "" {
			return nil, updateUsage("members", "desired group member LUID cannot be empty")
		}
		if seen[v] {
			return nil, updateUsage("members", "desired group membership contains a duplicate LUID")
		}
		seen[v] = true
	}
	slices.Sort(result)
	return result, nil
}
func membershipDiff(current []UpdateMember, desired []string) MembershipDiff {
	have := map[string]bool{}
	want := map[string]bool{}
	for _, v := range current {
		have[v.LUID] = true
	}
	for _, v := range desired {
		want[v] = true
	}
	d := MembershipDiff{}
	for _, v := range desired {
		if !have[v] {
			d.Add = append(d.Add, v)
		}
	}
	for v := range have {
		if !want[v] {
			d.Remove = append(d.Remove, v)
		}
	}
	slices.Sort(d.Remove)
	return d
}
func groupChanges(g UpdateGroup, r UpdateRequest) []UpdateChange {
	v := []UpdateChange{}
	if r.Name != nil && *r.Name != g.Name {
		v = append(v, UpdateChange{"name", g.Name, *r.Name})
	}
	if r.MinimumSiteRole != nil && *r.MinimumSiteRole != g.MinimumSiteRole {
		v = append(v, UpdateChange{"minimum_site_role", g.MinimumSiteRole, *r.MinimumSiteRole})
	}
	if r.ExternalUserEnabled != nil {
		before := ""
		if g.ExternalUserEnabled != nil && *g.ExternalUserEnabled {
			before = "true"
		} else if g.ExternalUserEnabled != nil {
			before = "false"
		}
		after := "false"
		if *r.ExternalUserEnabled {
			after = "true"
		}
		if before != after {
			v = append(v, UpdateChange{"external_user_enabled", before, after})
		}
	}
	return v
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateUpdateInput(in *UpdateInput) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return updateUsage("selector", "admin group update requires explicit environment and group LUID")
	}
	if in.Name == nil && in.MinimumSiteRole == nil && in.ExternalUserEnabled == nil && !in.MembershipSet {
		return updateUsage("fields", "admin group update requires metadata or an explicit desired membership")
	}
	desired, err := normalizeDesired(in.DesiredMemberLUIDs, in.MembershipSet)
	if err == nil {
		in.DesiredMemberLUIDs = desired
	}
	return err
}

// updateGroup preserves the original snapshot and its member identity projection.
func updateGroup(v Record) UpdateGroup {
	members := make([]UpdateMember, len(v.Members))
	for i, member := range v.Members {
		members[i] = UpdateMember{LUID: member.LUID, Name: member.Name}
	}
	return UpdateGroup{LUID: v.LUID, Name: v.Name, Domain: v.Domain, MinimumSiteRole: v.MinimumSiteRole, ExternalUserEnabled: v.ExternalUserEnabled, Members: members}
}
