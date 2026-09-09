package admin_test

import (
	"context"
	"testing"

	groupcreate "github.com/ahillspace/tadx/actions/admin/group/create"
	groupdelete "github.com/ahillspace/tadx/actions/admin/group/delete"
	groupupdate "github.com/ahillspace/tadx/actions/admin/group/update"
	userdelete "github.com/ahillspace/tadx/actions/admin/user/delete"
	userupdate "github.com/ahillspace/tadx/actions/admin/user/update"
)

type previewUserUpdate struct {
	unknownUserFake
	writes int
}

func (f *previewUserUpdate) UpdateUser(context.Context, string, userupdate.Request) (userupdate.User, error) {
	f.writes++
	return userupdate.User{}, nil
}

type previewUserDelete struct {
	unknownUserDeleteFake
	writes int
}

func (f *previewUserDelete) DeleteUser(context.Context, string) (userdelete.Result, error) {
	f.writes++
	return userdelete.Result{}, nil
}

type previewGroupCreate struct {
	unknownGroupFake
	writes int
}

func (f *previewGroupCreate) CreateGroup(context.Context, groupcreate.Request) (groupcreate.Group, error) {
	f.writes++
	return groupcreate.Group{}, nil
}

type previewGroupDelete struct {
	unknownGroupDeleteFake
	writes int
}

func (f *previewGroupDelete) DeleteGroup(context.Context, string) (groupdelete.Result, error) {
	f.writes++
	return groupdelete.Result{}, nil
}

func TestAdminPreviewDoesNotCallMutationDependencies(t *testing.T) {
	t.Run("user.update", func(t *testing.T) {
		f := &previewUserUpdate{}
		out, err := userupdate.New(f, f).Execute(context.Background(), userupdate.Input{Environment: "prod", Site: "site", UserLUID: "u1", FullName: sp("New name")}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("user.delete", func(t *testing.T) {
		f := &previewUserDelete{}
		out, err := userdelete.New(f, f).Execute(context.Background(), userdelete.Input{Environment: "prod", Site: "site", UserLUID: "u1"}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("group.create", func(t *testing.T) {
		f := &previewGroupCreate{}
		out, err := groupcreate.New(f, f).Execute(context.Background(), groupcreate.Input{Environment: "prod", Site: "site", Name: "Authors"}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("group.delete", func(t *testing.T) {
		f := &previewGroupDelete{}
		out, err := groupdelete.New(f, f).Execute(context.Background(), groupdelete.Input{Environment: "prod", Site: "site", GroupLUID: "g1"}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("group.update.metadata-and-membership", func(t *testing.T) {
		f := &groupUpdateFake{group: groupupdate.Group{LUID: "g1", Name: "Old", Members: []groupupdate.Member{{LUID: "u1"}}}}
		out, err := groupupdate.New(f, f, f).Execute(context.Background(), groupupdate.Input{Environment: "prod", Site: "site", GroupLUID: "g1", Name: sp("New"), MembershipSet: true, DesiredMemberLUIDs: []string{"u2"}}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || len(f.calls) != 0 {
			t.Fatalf("preview=%#v err=%v calls=%v", out, err, f.calls)
		}
	})
}
