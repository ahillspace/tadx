package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	groupops "github.com/ahillspace/tadx/actions/admin/group"
	permissionget "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"github.com/ahillspace/tadx/internal/errs"
	"testing"
)

type userListReader struct{}

func runUserList(ctx context.Context, reader userops.ListReader, input userops.ListInput) (userops.ListOutput, error) {
	if err := userops.ValidateListInput(&input); err != nil {
		return userops.ListOutput{}, err
	}
	if input.Cursor != "" {
		if err := userops.ValidateListContinuation(&input); err != nil {
			return userops.ListOutput{}, err
		}
	}
	return userops.List(ctx, reader, input)
}

func runGroupUpdate(ctx context.Context, resolver groupops.Resolver, writer groupops.UpdateWriter, members groupops.MembershipWriter, input groupops.UpdateInput, preview bool) (groupops.UpdateOutput, error) {
	if err := groupops.ValidateUpdateInput(&input); err != nil {
		return groupops.UpdateOutput{}, err
	}
	return groupops.Update(ctx, resolver, writer, members, input, preview)
}

func (userListReader) ListUsers(_ context.Context, in userops.ListPageRequest) (userops.ListPage, error) {
	return userops.ListPage{Number: in.PageNumber, Size: in.PageSize, Total: 26, Users: []userops.Record{{LUID: "u1", Name: "alex", SiteRole: "Viewer"}}}, nil
}

func TestMutationCompactProjectionsOmitRequestIDs(t *testing.T) {
	values := []interface {
		CompactOutput() any
		FullOutput() any
	}{
		groupops.CreateOutput{Plan: groupops.CreatePlan{Mode: "execute", Operation: "admin.group.create", Environment: "prod", Site: "site", Name: "Authors"}, Result: &groupops.CreateResult{Status: "created", GroupLUID: "g1", TableauRequestID: "secret-request"}},
		groupops.DeleteOutput{Plan: groupops.DeletePlan{Mode: "execute", Operation: "admin.group.delete", Environment: "prod", Site: "site", Target: groupops.DeleteGroup{LUID: "g1"}, PermissionImpact: "unknown"}, Result: &groupops.DeleteResult{Status: "deleted", GroupLUID: "g1", TableauRequestID: "secret-request"}},
		userops.DeleteOutput{Plan: userops.DeletePlan{Mode: "execute", Operation: "admin.user.delete", Environment: "prod", Site: "site", Target: userops.DeleteUser{LUID: "u1"}}, Result: &userops.DeleteResult{Status: "deleted", UserLUID: "u1", TableauRequestID: "secret-request"}},
		groupops.UpdateOutput{Plan: groupops.UpdatePlan{Mode: "execute", Operation: "admin.group.update", Environment: "prod", Site: "site", Target: groupops.UpdateGroup{LUID: "g1"}}, Result: &groupops.UpdateResult{Status: "updated", GroupLUID: "g1", TableauRequestIDs: []string{"secret-request"}}},
	}
	for i, value := range values {
		compact, _ := json.Marshal(value.CompactOutput())
		full, _ := json.Marshal(value.FullOutput())
		if string(compact) == "" || contains(string(compact), "secret-request") {
			t.Errorf("compact %d leaked request ID: %s", i, compact)
		}
		if !contains(string(full), "secret-request") {
			t.Errorf("full %d omitted request ID: %s", i, full)
		}
	}
}
func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

type partialGroupUpdateFake struct{ calls int }

func (*partialGroupUpdateFake) ResolveGroup(context.Context, groupops.Selector, bool) (groupops.Record, error) {
	return groupops.Record{LUID: "g1", Name: "Authors"}, nil
}
func (*partialGroupUpdateFake) UpdateGroup(context.Context, string, groupops.UpdateRequest) (groupops.Record, error) {
	return groupops.Record{}, nil
}
func (f *partialGroupUpdateFake) AddGroupUser(_ context.Context, _, user string) (string, error) {
	f.calls++
	if f.calls == 2 {
		return "", errors.New("second member failed")
	}
	return "request-" + user, nil
}
func (*partialGroupUpdateFake) RemoveGroupUser(context.Context, string, string) (string, error) {
	return "", nil
}
func TestGroupUpdateReportsExactPartialProgress(t *testing.T) {
	fake := &partialGroupUpdateFake{}
	_, err := runGroupUpdate(context.Background(), fake, fake, fake, groupops.UpdateInput{Environment: "prod", Site: "site", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u1", "u2"}}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %v", err)
	}
	if structured.ID != "admin.group.update.partial" || structured.Retryable == nil || *structured.Retryable || len(structured.Completed) != 1 || structured.Completed[0] != "member.add:u1" || structured.Failed != "member.add:u2" {
		t.Fatalf("structured = %#v", structured)
	}
}
func TestUserListIsBoundedAndAdvertisesFull(t *testing.T) {
	out, err := runUserList(context.Background(), userListReader{}, userops.ListInput{Limit: 25})
	if err != nil || out.Page.Returned != 1 || out.Page.NextCursor == "" {
		t.Fatalf("Execute() = %#v, %v", out, err)
	}
	if out.CompactOutput().(userops.ListCompactResult).Details != "--full" {
		t.Fatal("compact output omitted --full disclosure")
	}
}

type userCreateFake struct{ created int }

func (f *userCreateFake) UserExists(context.Context, string) (bool, error) {
	return false, nil

}
func (f *userCreateFake) CreateUser(_ context.Context, r userops.CreateRequest) (userops.Record, error) {
	f.created++
	return userops.Record{LUID: "u1", Name: r.Name, SiteRole: r.SiteRole, RequestID: "request-1"}, nil
}
func TestUserCreateSupportsExplicitPreview(t *testing.T) {
	fake := &userCreateFake{}

	in := userops.CreateInput{Environment: "prod", Site: "site", Name: "alex@example.com", SiteRole: "Viewer", AuthSetting: "SAML"}
	out, err := userops.Create(context.Background(), fake, fake, in, true)
	if err != nil || out.Result != nil || fake.created != 0 {
		t.Fatalf("preview = %#v, %v, calls %d", out, err, fake.created)
	}
	out, err = userops.Create(context.Background(), fake, fake, in, false)
	if err != nil || out.Result == nil || fake.created != 1 {
		t.Fatalf("result = %#v, %v, calls %d", out, err, fake.created)
	}
}

type groupUpdateFake struct {
	group groupops.Record
	calls []string
}

func (f *groupUpdateFake) ResolveGroup(context.Context, groupops.Selector, bool) (groupops.Record, error) {
	return f.group, nil
}
func (f *groupUpdateFake) UpdateGroup(context.Context, string, groupops.UpdateRequest) (groupops.Record, error) {
	f.calls = append(f.calls, "metadata")
	return f.group, nil
}
func (f *groupUpdateFake) AddGroupUser(_ context.Context, _, u string) (string, error) {
	f.calls = append(f.calls, "add:"+u)
	return "add-request", nil
}
func (f *groupUpdateFake) RemoveGroupUser(_ context.Context, _, u string) (string, error) {
	f.calls = append(f.calls, "remove:"+u)
	return "remove-request", nil
}
func TestGroupUpdatePlansAndOrdersMembershipDiff(t *testing.T) {
	fake := &groupUpdateFake{group: groupops.Record{LUID: "g1", Name: "Authors", Members: []groupops.Member{{LUID: "u2"}, {LUID: "u1"}}}}

	out, err := runGroupUpdate(context.Background(), fake, fake, fake, groupops.UpdateInput{Environment: "prod", Site: "site", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u2", "u3"}}, true)
	if err != nil || out.Plan.Membership == nil || len(out.Plan.Membership.Add) != 1 || len(out.Plan.Membership.Remove) != 1 || len(fake.calls) != 0 {
		t.Fatalf("preview = %#v, %v, calls %#v", out, err, fake.calls)
	}
	out, err = runGroupUpdate(context.Background(), fake, fake, fake, groupops.UpdateInput{Environment: "prod", Site: "site", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u2", "u3"}}, false)
	if err != nil || out.Result == nil || len(fake.calls) != 2 || fake.calls[0] != "add:u3" || fake.calls[1] != "remove:u1" {
		t.Fatalf("result = %#v, %v, calls %#v", out, err, fake.calls)
	}
}

func TestUserCreatePlanReflectsAppliedFields(t *testing.T) {
	fake := &userCreateFake{}
	in := userops.CreateInput{Environment: "prod", Site: "site", Name: "alex@example.com", SiteRole: "Viewer", AuthSetting: "SAML", IdentityPoolName: "pool-a", Email: "notify@example.com", Language: "en", Locale: "en_US"}
	out, err := userops.Create(context.Background(), fake, fake, in, true)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, projection := range []any{out.CompactOutput(), out.FullOutput()} {
		data, _ := json.Marshal(projection)
		for _, want := range []string{`"identity_pool_name":"pool-a"`, `"email":"notify@example.com"`, `"language":"en"`, `"locale":"en_US"`} {
			if !contains(string(data), want) {
				t.Errorf("projection %s omitted %s", data, want)
			}
		}
	}
}

type unknownUserFake struct{}

func (unknownUserFake) UserExists(context.Context, string) (bool, error) {
	return false, nil

}
func (unknownUserFake) CreateUser(context.Context, userops.CreateRequest) (userops.Record, error) {
	return userops.Record{RequestID: "req-c", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}
func (unknownUserFake) ResolveUser(context.Context, userops.Selector) (userops.Record, error) {
	return userops.Record{LUID: "u1"}, nil
}
func (unknownUserFake) UpdateUser(context.Context, string, userops.UpdateRequest) (userops.Record, error) {
	return userops.Record{LUID: "u1", RequestID: "req-u", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}

type unknownUserDeleteFake struct{}

func (unknownUserDeleteFake) ResolveUser(context.Context, userops.Selector) (userops.Record, error) {
	return userops.Record{LUID: "u1"}, nil
}
func (unknownUserDeleteFake) DeleteUser(context.Context, string) (userops.DeleteResult, error) {
	return userops.DeleteResult{Status: "unknown", UserLUID: "u1", TableauRequestID: "req-d"}, errors.New("delete outcome uncertain")
}

func sp(value string) *string { return &value }

func assertUnknownOutcome(t *testing.T, err error, wantID, wantRequestID, wantResource string) {
	t.Helper()
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %v (not structured)", err)
	}
	if structured.ID != wantID || structured.Kind != errs.KindOperation || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("structured = %#v", structured)
	}
	if structured.TableauRequestID != wantRequestID || structured.Resource != wantResource {
		t.Fatalf("structured identity = %#v", structured)
	}
}

func TestAdminMutationsSurfaceUnknownOutcome(t *testing.T) {
	uf := unknownUserFake{}
	_, err := userops.Create(context.Background(), uf, uf, userops.CreateInput{Environment: "prod", Site: "site", Name: "alex", SiteRole: "Viewer", AuthSetting: "SAML"}, false)
	assertUnknownOutcome(t, err, "admin.user.create.outcome_unknown", "req-c", "alex")

	full := sp("Alex")
	_, err = userops.Update(context.Background(), uf, uf, userops.UpdateInput{Environment: "prod", Site: "site", UserLUID: "u1", FullName: full}, false)
	assertUnknownOutcome(t, err, "admin.user.update.outcome_unknown", "req-u", "u1")

	df := unknownUserDeleteFake{}
	_, err = userops.Delete(context.Background(), df, df, userops.DeleteInput{Environment: "prod", Site: "site", UserLUID: "u1"}, false)
	assertUnknownOutcome(t, err, "admin.user.delete.outcome_unknown", "req-d", "u1")

	gc := unknownGroupFake{}
	_, err = groupops.Create(context.Background(), gc, gc, groupops.CreateInput{Environment: "prod", Site: "site", Name: "Authors"}, false)
	assertUnknownOutcome(t, err, "admin.group.create.outcome_unknown", "req-gc", "Authors")

	gu := &unknownGroupUpdateFake{}
	_, err = runGroupUpdate(context.Background(), gu, gu, gu, groupops.UpdateInput{Environment: "prod", Site: "site", GroupLUID: "g1", Name: sp("New")}, false)
	assertUnknownOutcome(t, err, "admin.group.update.outcome_unknown", "req-gu", "g1")

	gd := unknownGroupDeleteFake{}
	_, err = groupops.Delete(context.Background(), gd, gd, groupops.DeleteInput{Environment: "prod", Site: "site", GroupLUID: "g1"}, false)
	assertUnknownOutcome(t, err, "admin.group.delete.outcome_unknown", "req-gd", "g1")
}

type unknownGroupFake struct{}

func (unknownGroupFake) GroupExists(context.Context, string) (bool, error) {
	return false, nil

}
func (unknownGroupFake) CreateGroup(context.Context, groupops.CreateRequest) (groupops.Record, error) {
	return groupops.Record{RequestID: "req-gc", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}

type unknownGroupUpdateFake struct{}

func (*unknownGroupUpdateFake) ResolveGroup(context.Context, groupops.Selector, bool) (groupops.Record, error) {
	return groupops.Record{LUID: "g1", Name: "Old"}, nil
}
func (*unknownGroupUpdateFake) UpdateGroup(context.Context, string, groupops.UpdateRequest) (groupops.Record, error) {
	return groupops.Record{LUID: "g1", RequestID: "req-gu", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}
func (*unknownGroupUpdateFake) AddGroupUser(context.Context, string, string) (string, error) {
	return "", nil
}
func (*unknownGroupUpdateFake) RemoveGroupUser(context.Context, string, string) (string, error) {
	return "", nil
}

type unknownGroupDeleteFake struct{}

func (unknownGroupDeleteFake) ResolveGroup(context.Context, groupops.Selector, bool) (groupops.Record, error) {
	return groupops.Record{LUID: "g1"}, nil
}
func (unknownGroupDeleteFake) DeleteGroup(context.Context, string) (groupops.DeleteResult, error) {
	return groupops.DeleteResult{Status: "unknown", GroupLUID: "g1", TableauRequestID: "req-gd"}, errors.New("delete outcome uncertain")
}

func TestAdminUsageValidationIsKindUsage(t *testing.T) {
	cases := []func() error{
		func() error {
			return userops.ValidateCreateInput(userops.CreateInput{Environment: "prod", Site: "site", Name: "a", SiteRole: "Viewer", AuthSetting: "SAML", IdPConfigurationID: "idp-1"})
		},
		func() error {
			return userops.ValidateCreateInput(userops.CreateInput{Environment: "prod", Site: "site", Name: "a"})
		},
		func() error {
			return userops.ValidateUpdateInput(userops.UpdateInput{Environment: "prod", Site: "site", UserLUID: "u1"})
		},
		func() error {
			return userops.ValidateUpdateInput(userops.UpdateInput{Environment: "prod", Site: "site", UserLUID: "u1", AuthSetting: sp("SAML"), IdPConfigurationID: sp("idp-1")})
		},
		func() error {
			_, err := runUserList(context.Background(), userListReader{}, userops.ListInput{Limit: 10001})
			return err
		},
	}
	for i, run := range cases {
		err := run()
		var structured *errs.Error
		if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || errs.ExitCode(err) != 2 {
			t.Errorf("case %d error = %v, exit = %d", i, err, errs.ExitCode(err))
		}
	}
}

type permissionReader struct{}

func (permissionReader) GetPermissions(_ context.Context, in permissionget.Input) (permissionget.PermissionSet, error) {
	return permissionget.PermissionSet{ResourceKind: in.ResourceKind, ResourceLUID: in.ResourceLUID, Source: "direct", Rules: []permissionget.Rule{{PrincipalType: "group", PrincipalLUID: "g1", Capability: "Read", Mode: "Allow"}, {PrincipalType: "user", PrincipalLUID: "u1", Capability: "Write", Mode: "Deny"}}}, nil
}
func TestPermissionGetFiltersWithoutClaimingEffectiveAccess(t *testing.T) {
	out, err := permissionget.Inspect(context.Background(), permissionReader{}, permissionget.Input{ResourceKind: "workbook", ResourceLUID: "w1", PrincipalType: "group"})
	if err != nil || len(out.Permissions.Rules) != 1 || out.Permissions.Rules[0].Source != "direct" {
		t.Fatalf("Execute() = %#v, %v", out, err)
	}
}
