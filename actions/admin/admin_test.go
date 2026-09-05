package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	groupcreate "github.com/ahillspace/tadx/actions/admin/group/create"
	groupdelete "github.com/ahillspace/tadx/actions/admin/group/delete"
	groupupdate "github.com/ahillspace/tadx/actions/admin/group/update"
	permissionget "github.com/ahillspace/tadx/actions/admin/permission/inspect"
	usercreate "github.com/ahillspace/tadx/actions/admin/user/create"
	userdelete "github.com/ahillspace/tadx/actions/admin/user/delete"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	userupdate "github.com/ahillspace/tadx/actions/admin/user/update"
	"github.com/ahillspace/tadx/internal/errs"
)

type userListReader struct{}

func (userListReader) ListUsers(_ context.Context, in userlist.PageRequest) (userlist.Page, error) {
	return userlist.Page{Number: in.PageNumber, Size: in.PageSize, Total: 26, Users: []userlist.User{{LUID: "u1", Name: "alex", SiteRole: "Viewer"}}}, nil
}

func TestMutationCompactProjectionsOmitRequestIDs(t *testing.T) {
	values := []interface {
		CompactOutput() any
		FullOutput() any
	}{
		groupcreate.Output{Plan: groupcreate.Plan{Mode: "execute", Operation: "admin.group.create", Environment: "prod", Site: "site", Name: "Authors"}, Result: &groupcreate.Result{Status: "created", GroupLUID: "g1", TableauRequestID: "secret-request"}},
		groupdelete.Output{Plan: groupdelete.Plan{Mode: "execute", Operation: "admin.group.delete", Environment: "prod", Site: "site", Target: groupdelete.Group{LUID: "g1"}, PermissionImpact: "unknown"}, Result: &groupdelete.Result{Status: "deleted", GroupLUID: "g1", TableauRequestID: "secret-request"}},
		userdelete.Output{Plan: userdelete.Plan{Mode: "execute", Operation: "admin.user.delete", Environment: "prod", Site: "site", Target: userdelete.User{LUID: "u1"}}, Result: &userdelete.Result{Status: "deleted", UserLUID: "u1", TableauRequestID: "secret-request"}},
		groupupdate.Output{Plan: groupupdate.Plan{Mode: "execute", Operation: "admin.group.update", Environment: "prod", Site: "site", Target: groupupdate.Group{LUID: "g1"}}, Result: &groupupdate.Result{Status: "updated", GroupLUID: "g1", TableauRequestIDs: []string{"secret-request"}}},
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

func (*partialGroupUpdateFake) ResolveGroup(context.Context, string, bool) (groupupdate.Group, error) {
	return groupupdate.Group{LUID: "g1", Name: "Authors"}, nil
}
func (*partialGroupUpdateFake) UpdateGroup(context.Context, string, groupupdate.Request) (groupupdate.Group, error) {
	return groupupdate.Group{}, nil
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
	_, err := groupupdate.New(fake, fake, fake).Execute(context.Background(), groupupdate.Input{Environment: "prod", Site: "site", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u1", "u2"}}, false)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %v", err)
	}
	if structured.ID != "admin.group.update.partial" || structured.Retryable == nil || *structured.Retryable || len(structured.Completed) != 1 || structured.Completed[0] != "member.add:u1" || structured.Failed != "member.add:u2" {
		t.Fatalf("structured = %#v", structured)
	}
}
func TestUserListIsBoundedAndAdvertisesFull(t *testing.T) {
	out, err := userlist.New(userListReader{}).Execute(context.Background(), userlist.Input{Limit: 25})
	if err != nil || out.Page.Returned != 1 || out.Page.NextCursor == "" {
		t.Fatalf("Execute() = %#v, %v", out, err)
	}
	if out.CompactOutput().(userlist.CompactResult).Details != "--full" {
		t.Fatal("compact output omitted --full disclosure")
	}
}

type userCreateFake struct{ created int }

func (f *userCreateFake) FindUsers(context.Context, string) ([]usercreate.User, error) {
	return nil, nil
}
func (f *userCreateFake) CreateUser(_ context.Context, r usercreate.Request) (usercreate.User, error) {
	f.created++
	return usercreate.User{LUID: "u1", Name: r.Name, SiteRole: r.SiteRole, RequestID: "request-1"}, nil
}
func TestUserCreateSupportsExplicitPreview(t *testing.T) {
	fake := &userCreateFake{}
	action := usercreate.New(fake, fake)
	in := usercreate.Input{Environment: "prod", Site: "site", Name: "alex@example.com", SiteRole: "Viewer", AuthSetting: "SAML"}
	out, err := action.Execute(context.Background(), in, true)
	if err != nil || out.Result != nil || fake.created != 0 {
		t.Fatalf("preview = %#v, %v, calls %d", out, err, fake.created)
	}
	out, err = action.Execute(context.Background(), in, false)
	if err != nil || out.Result == nil || fake.created != 1 {
		t.Fatalf("result = %#v, %v, calls %d", out, err, fake.created)
	}
}

type groupUpdateFake struct {
	group groupupdate.Group
	calls []string
}

func (f *groupUpdateFake) ResolveGroup(context.Context, string, bool) (groupupdate.Group, error) {
	return f.group, nil
}
func (f *groupUpdateFake) UpdateGroup(context.Context, string, groupupdate.Request) (groupupdate.Group, error) {
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
	fake := &groupUpdateFake{group: groupupdate.Group{LUID: "g1", Name: "Authors", Members: []groupupdate.Member{{LUID: "u2"}, {LUID: "u1"}}}}
	action := groupupdate.New(fake, fake, fake)
	out, err := action.Execute(context.Background(), groupupdate.Input{Environment: "prod", Site: "site", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u2", "u3"}}, true)
	if err != nil || out.Plan.Membership == nil || len(out.Plan.Membership.Add) != 1 || len(out.Plan.Membership.Remove) != 1 || len(fake.calls) != 0 {
		t.Fatalf("preview = %#v, %v, calls %#v", out, err, fake.calls)
	}
	out, err = action.Execute(context.Background(), groupupdate.Input{Environment: "prod", Site: "site", GroupLUID: "g1", MembershipSet: true, DesiredMemberLUIDs: []string{"u2", "u3"}}, false)
	if err != nil || out.Result == nil || len(fake.calls) != 2 || fake.calls[0] != "add:u3" || fake.calls[1] != "remove:u1" {
		t.Fatalf("result = %#v, %v, calls %#v", out, err, fake.calls)
	}
}

func TestUserCreatePlanReflectsAppliedFields(t *testing.T) {
	fake := &userCreateFake{}
	in := usercreate.Input{Environment: "prod", Site: "site", Name: "alex@example.com", SiteRole: "Viewer", AuthSetting: "SAML", IdentityPoolName: "pool-a", Email: "notify@example.com", Language: "en", Locale: "en_US"}
	out, err := usercreate.New(fake, fake).Execute(context.Background(), in, true)
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

func (unknownUserFake) FindUsers(context.Context, string) ([]usercreate.User, error) { return nil, nil }
func (unknownUserFake) CreateUser(context.Context, usercreate.Request) (usercreate.User, error) {
	return usercreate.User{RequestID: "req-c", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}
func (unknownUserFake) ResolveUser(context.Context, string) (userupdate.User, error) {
	return userupdate.User{LUID: "u1"}, nil
}
func (unknownUserFake) UpdateUser(context.Context, string, userupdate.Request) (userupdate.User, error) {
	return userupdate.User{LUID: "u1", RequestID: "req-u", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}

type unknownUserDeleteFake struct{}

func (unknownUserDeleteFake) ResolveUser(context.Context, string) (userdelete.User, error) {
	return userdelete.User{LUID: "u1"}, nil
}
func (unknownUserDeleteFake) DeleteUser(context.Context, string) (userdelete.Result, error) {
	return userdelete.Result{Status: "unknown", UserLUID: "u1", TableauRequestID: "req-d"}, errors.New("delete outcome uncertain")
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
	_, err := usercreate.New(uf, uf).Execute(context.Background(), usercreate.Input{Environment: "prod", Site: "site", Name: "alex", SiteRole: "Viewer", AuthSetting: "SAML"}, false)
	assertUnknownOutcome(t, err, "admin.user.create.outcome_unknown", "req-c", "")

	full := sp("Alex")
	_, err = userupdate.New(uf, uf).Execute(context.Background(), userupdate.Input{Environment: "prod", Site: "site", UserLUID: "u1", FullName: full}, false)
	assertUnknownOutcome(t, err, "admin.user.update.outcome_unknown", "req-u", "u1")

	df := unknownUserDeleteFake{}
	_, err = userdelete.New(df, df).Execute(context.Background(), userdelete.Input{Environment: "prod", Site: "site", UserLUID: "u1"}, false)
	assertUnknownOutcome(t, err, "admin.user.delete.outcome_unknown", "req-d", "u1")

	gc := unknownGroupFake{}
	_, err = groupcreate.New(gc, gc).Execute(context.Background(), groupcreate.Input{Environment: "prod", Site: "site", Name: "Authors"}, false)
	assertUnknownOutcome(t, err, "admin.group.create.outcome_unknown", "req-gc", "")

	gu := &unknownGroupUpdateFake{}
	_, err = groupupdate.New(gu, gu, gu).Execute(context.Background(), groupupdate.Input{Environment: "prod", Site: "site", GroupLUID: "g1", Name: sp("New")}, false)
	assertUnknownOutcome(t, err, "admin.group.update.outcome_unknown", "req-gu", "g1")

	gd := unknownGroupDeleteFake{}
	_, err = groupdelete.New(gd, gd).Execute(context.Background(), groupdelete.Input{Environment: "prod", Site: "site", GroupLUID: "g1"}, false)
	assertUnknownOutcome(t, err, "admin.group.delete.outcome_unknown", "req-gd", "g1")
}

type unknownGroupFake struct{}

func (unknownGroupFake) FindGroups(context.Context, string) ([]groupcreate.Group, error) {
	return nil, nil
}
func (unknownGroupFake) CreateGroup(context.Context, groupcreate.Request) (groupcreate.Group, error) {
	return groupcreate.Group{RequestID: "req-gc", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}

type unknownGroupUpdateFake struct{}

func (*unknownGroupUpdateFake) ResolveGroup(context.Context, string, bool) (groupupdate.Group, error) {
	return groupupdate.Group{LUID: "g1", Name: "Old"}, nil
}
func (*unknownGroupUpdateFake) UpdateGroup(context.Context, string, groupupdate.Request) (groupupdate.Group, error) {
	return groupupdate.Group{LUID: "g1", RequestID: "req-gu", MutationStatus: "unknown"}, errors.New("post-mutation status mismatch")
}
func (*unknownGroupUpdateFake) AddGroupUser(context.Context, string, string) (string, error) {
	return "", nil
}
func (*unknownGroupUpdateFake) RemoveGroupUser(context.Context, string, string) (string, error) {
	return "", nil
}

type unknownGroupDeleteFake struct{}

func (unknownGroupDeleteFake) ResolveGroup(context.Context, string) (groupdelete.Group, error) {
	return groupdelete.Group{LUID: "g1"}, nil
}
func (unknownGroupDeleteFake) DeleteGroup(context.Context, string) (groupdelete.Result, error) {
	return groupdelete.Result{Status: "unknown", GroupLUID: "g1", TableauRequestID: "req-gd"}, errors.New("delete outcome uncertain")
}

func TestAdminUsageValidationIsKindUsage(t *testing.T) {
	uf := &userCreateFake{}
	cases := []func() error{
		func() error {
			_, err := usercreate.New(uf, uf).Execute(context.Background(), usercreate.Input{Environment: "prod", Site: "site", Name: "a", SiteRole: "Viewer", AuthSetting: "SAML", IdPConfigurationID: "idp-1"}, false)
			return err
		},
		func() error {
			_, err := usercreate.New(uf, uf).Execute(context.Background(), usercreate.Input{Environment: "prod", Site: "site", Name: "a"}, false)
			return err
		},
		func() error {
			f := unknownUserFake{}
			_, err := userupdate.New(f, f).Execute(context.Background(), userupdate.Input{Environment: "prod", Site: "site", UserLUID: "u1"}, false)
			return err
		},
		func() error {
			f := unknownUserFake{}
			_, err := userupdate.New(f, f).Execute(context.Background(), userupdate.Input{Environment: "prod", Site: "site", UserLUID: "u1", AuthSetting: sp("SAML"), IdPConfigurationID: sp("idp-1")}, false)
			return err
		},
		func() error {
			_, err := userlist.New(userListReader{}).Execute(context.Background(), userlist.Input{Limit: 5000})
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
	out, err := permissionget.New(permissionReader{}).Execute(context.Background(), permissionget.Input{ResourceKind: "workbook", ResourceLUID: "w1", PrincipalType: "group"})
	if err != nil || len(out.Permissions.Rules) != 1 || out.Permissions.Rules[0].Source != "direct" {
		t.Fatalf("Execute() = %#v, %v", out, err)
	}
}
