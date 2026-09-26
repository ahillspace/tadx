package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"reflect"
	"sort"
	"strings"
)

const maxDesiredMembers = 1000

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved               bool
	Environment, Site, GroupLUID string
	Name, MinimumSiteRole        *string
	ExternalUserEnabled          *bool
	MembershipSet                bool
	DesiredMemberLUIDs           []string
}
type Member struct {
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
}
type Group struct {
	LUID                string   `json:"luid"`
	Name                string   `json:"name"`
	Domain              string   `json:"domain,omitempty"`
	MinimumSiteRole     string   `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool    `json:"external_user_enabled,omitempty"`
	Members             []Member `json:"members,omitempty"`
	RequestID           string   `json:"-"`
	MutationStatus      string   `json:"-"`
}
type Request struct {
	Name                *string `json:"name,omitempty"`
	MinimumSiteRole     *string `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool   `json:"external_user_enabled,omitempty"`
}
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type MembershipDiff struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}
type Plan struct {
	Mode        string          `json:"mode"`
	Operation   string          `json:"operation"`
	Environment string          `json:"environment"`
	Site        string          `json:"site"`
	Target      Group           `json:"target"`
	Requested   Request         `json:"requested"`
	Changes     []Change        `json:"changes"`
	Membership  *MembershipDiff `json:"membership,omitempty"`
	NoOp        bool            `json:"no_op"`
}
type Result struct {
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
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactPlan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	GroupLUID   string   `json:"group_luid"`
	Requested   *Request `json:"requested,omitempty"`
	Changes     []Change `json:"changes,omitempty"`
	ChangeCount int      `json:"change_count"`
	AddCount    int      `json:"add_count"`
	RemoveCount int      `json:"remove_count"`
	NoOp        bool     `json:"no_op"`
}
type CompactResult struct {
	Plan    CompactPlan            `json:"plan"`
	Result  *CompactMutationResult `json:"result,omitempty"`
	Details string                 `json:"details"`
	Help    []string               `json:"help"`
}
type CompactMutationResult struct {
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

func (o Output) CompactOutput() any {
	add, remove := 0, 0
	if o.Plan.Membership != nil {
		add, remove = len(o.Plan.Membership.Add), len(o.Plan.Membership.Remove)
	}
	var result *CompactMutationResult
	if o.Result != nil {
		result = &CompactMutationResult{Status: o.Result.Status, GroupLUID: o.Result.GroupLUID, Added: o.Result.Added, Removed: o.Result.Removed}
		result.Group = o.Result.Group
		result.AddedUserLUIDs, result.AddedUserLUIDsOmitted = boundedMemberIDs(o.Result.AddedUserLUIDs)
		result.RemovedUserLUIDs, result.RemovedUserLUIDsOmitted = boundedMemberIDs(o.Result.RemovedUserLUIDs)
	}
	plan := CompactPlan{Mode: o.Plan.Mode, Operation: o.Plan.Operation, Environment: o.Plan.Environment, Site: o.Plan.Site, GroupLUID: o.Plan.Target.LUID, ChangeCount: len(o.Plan.Changes), AddCount: add, RemoveCount: remove, NoOp: o.Plan.NoOp}
	if o.Plan.Mode == "preview" {
		plan.Requested = new(o.Plan.Requested)
		plan.Changes = o.Plan.Changes
	}
	return CompactResult{Plan: plan, Result: result, Details: "--full", Help: o.Help}
}
func boundedMemberIDs(ids []string) ([]string, int) {
	const limit = 10
	if len(ids) > limit {
		return append([]string(nil), ids[:limit]...), len(ids) - limit
	}
	return append([]string(nil), ids...), 0
}
func (o Output) FullOutput() any {
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

type Resolver interface {
	ResolveGroup(context.Context, string, bool) (Group, error)
}
type Updater interface {
	UpdateGroup(context.Context, string, Request) (Group, error)
}
type MembershipWriter interface {
	AddGroupUser(context.Context, string, string) (string, error)
	RemoveGroupUser(context.Context, string, string) (string, error)
}
type Action struct {
	resolver Resolver
	updater  Updater
	members  MembershipWriter
}

func New(r Resolver, u Updater, m MembershipWriter) *Action {
	return &Action{resolver: r, updater: u, members: m}
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.updater == nil || a.members == nil {
		return Output{}, errors.New("admin group update is not configured")
	}
	desired, err := validateInput(in)
	if err != nil {
		return Output{}, err
	}
	if in.Site == "" && !in.TargetResolved {
		return Output{}, usage("selector", "admin group update requires explicit environment, site, and group LUID")
	}
	group, err := a.resolver.ResolveGroup(ctx, in.GroupLUID, in.MembershipSet)
	if err != nil {
		return Output{}, err
	}
	if in.MembershipSet && len(group.Members) > maxDesiredMembers {
		return Output{}, errors.New("current group membership exceeds the 1000-member action bound")
	}
	changes := groupChanges(group, Request{in.Name, in.MinimumSiteRole, in.ExternalUserEnabled})
	var membership *MembershipDiff
	if in.MembershipSet {
		diff := membershipDiff(group.Members, desired)
		membership = &diff
	}
	noOp := len(changes) == 0 && (membership == nil || (len(membership.Add) == 0 && len(membership.Remove) == 0))
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.group.update", Environment: in.Environment, Site: in.Site, Target: group, Requested: Request{in.Name, in.MinimumSiteRole, in.ExternalUserEnabled}, Changes: changes, Membership: membership, NoOp: noOp}, Help: []string{"Run without --preview to update this exact group and converge direct membership."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.resolver.ResolveGroup(ctx, in.GroupLUID, in.MembershipSet)
	if err != nil {
		return Output{}, err
	}
	if !reflect.DeepEqual(current, group) {
		return Output{}, errors.New("the group or its direct membership changed during revalidation")
	}
	result := &Result{Status: "unchanged", GroupLUID: group.LUID}
	out.Result = result
	if noOp {
		return out, nil
	}
	if len(changes) > 0 {
		updated, err := a.updater.UpdateGroup(ctx, group.LUID, Request{in.Name, in.MinimumSiteRole, in.ExternalUserEnabled})
		if err != nil {
			if updated.MutationStatus == "unknown" {
				return Output{}, outcomeUnknown(in, group.LUID, updated.RequestID, err)
			}
			return Output{}, err
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
			id, err := a.members.AddGroupUser(ctx, group.LUID, luid)
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
			id, err := a.members.RemoveGroupUser(ctx, group.LUID, luid)
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

func usage(field, message string) error {
	return &errs.Error{ID: "admin.group.update.usage", Kind: errs.KindUsage, Operation: "admin.group.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the group update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func outcomeUnknown(in Input, luid, requestID string, cause error) error {
	return &errs.Error{ID: "admin.group.update.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.update", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The group update outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact group and Tableau request before retrying: " + commandhint.Environment(in.Environment, "admin", "group", "inspect", "--id", luid), TableauRequestID: requestID}
}
func partialError(input Input, groupLUID string, completed []string, failed string, cause error) error {
	return &errs.Error{ID: "admin.group.update.partial", Kind: errs.KindOperation, Operation: "admin.group.update", Resource: groupLUID, Environment: input.Environment, Site: input.Site, Summary: "Group update stopped after a partial remote mutation.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Re-read the exact group membership before reviewing a new update plan: " + commandhint.Environment(input.Environment, "admin", "group", "inspect", "--id", groupLUID, "--members"), Completed: append([]string(nil), completed...), Failed: failed, TableauRequestID: errs.TableauRequestID(cause)}
}
func normalizeDesired(values []string, enabled bool) ([]string, error) {
	if !enabled {
		return nil, nil
	}
	if len(values) > maxDesiredMembers {
		return nil, usage("members", "desired group membership exceeds the 1000-member action bound")
	}
	seen := map[string]bool{}
	result := append([]string(nil), values...)
	for _, v := range result {
		if v == "" {
			return nil, usage("members", "desired group member LUID cannot be empty")
		}
		if seen[v] {
			return nil, usage("members", "desired group membership contains a duplicate LUID")
		}
		seen[v] = true
	}
	sort.Strings(result)
	return result, nil
}
func membershipDiff(current []Member, desired []string) MembershipDiff {
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
	sort.Strings(d.Remove)
	return d
}
func groupChanges(g Group, r Request) []Change {
	v := []Change{}
	if r.Name != nil && *r.Name != g.Name {
		v = append(v, Change{"name", g.Name, *r.Name})
	}
	if r.MinimumSiteRole != nil && *r.MinimumSiteRole != g.MinimumSiteRole {
		v = append(v, Change{"minimum_site_role", g.MinimumSiteRole, *r.MinimumSiteRole})
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
			v = append(v, Change{"external_user_enabled", before, after})
		}
	}
	return v
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	_, err := validateInput(in)
	return err
}

func validateInput(in Input) ([]string, error) {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return nil, usage("selector", "admin group update requires explicit environment and group LUID")
	}
	if in.Name == nil && in.MinimumSiteRole == nil && in.ExternalUserEnabled == nil && !in.MembershipSet {
		return nil, usage("fields", "admin group update requires metadata or an explicit desired membership")
	}
	return normalizeDesired(in.DesiredMemberLUIDs, in.MembershipSet)
}
