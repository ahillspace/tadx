package admin_test

import (
	"context"
	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	"testing"
)

type previewUserUpdate struct {
	unknownUserFake
	writes int
}

func (f *previewUserUpdate) UpdateUser(context.Context, string, userops.UpdateRequest) (userops.Record, error) {
	f.writes++
	return userops.Record{}, nil
}

type previewUserDelete struct {
	unknownUserDeleteFake
	writes int
}

func (f *previewUserDelete) DeleteUser(context.Context, string) (userops.DeleteResult, error) {
	f.writes++
	return userops.DeleteResult{}, nil
}

type previewGroupCreate struct {
	unknownGroupFake
	writes int
}

func (f *previewGroupCreate) CreateGroup(context.Context, groupops.CreateRequest) (groupops.Record, error) {
	f.writes++
	return groupops.Record{}, nil
}

type previewGroupDelete struct {
	unknownGroupDeleteFake
	writes int
}

func (f *previewGroupDelete) DeleteGroup(context.Context, string) (groupops.DeleteResult, error) {
	f.writes++
	return groupops.DeleteResult{}, nil
}

func TestAdminPreviewDoesNotCallMutationDependencies(t *testing.T) {
	t.Run("user.update", func(t *testing.T) {
		f := &previewUserUpdate{}
		out, err := userops.Update(context.Background(), f, f, userops.UpdateInput{Environment: "prod", Site: "site", UserLUID: "u1", FullName: sp("New name")}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("user.delete", func(t *testing.T) {
		f := &previewUserDelete{}
		out, err := userops.Delete(context.Background(), f, f, userops.DeleteInput{Environment: "prod", Site: "site", UserLUID: "u1"}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("group.create", func(t *testing.T) {
		f := &previewGroupCreate{}
		out, err := groupops.Create(context.Background(), f, f, groupops.CreateInput{Environment: "prod", Site: "site", Name: "Authors"}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("group.delete", func(t *testing.T) {
		f := &previewGroupDelete{}
		out, err := groupops.Delete(context.Background(), f, f, groupops.DeleteInput{Environment: "prod", Site: "site", GroupLUID: "g1"}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || f.writes != 0 {
			t.Fatalf("preview=%#v err=%v writes=%d", out, err, f.writes)
		}
	})
	t.Run("group.update.metadata-and-membership", func(t *testing.T) {
		f := &groupUpdateFake{group: groupops.Record{LUID: "g1", Name: "Old", Members: []groupops.Member{{LUID: "u1"}}}}
		out, err := runGroupUpdate(context.Background(), f, f, f, groupops.UpdateInput{Environment: "prod", Site: "site", GroupLUID: "g1", Name: sp("New"), MembershipSet: true, DesiredMemberLUIDs: []string{"u2"}}, true)
		if err != nil || out.Plan.Mode != "preview" || out.Result != nil || len(f.calls) != 0 {
			t.Fatalf("preview=%#v err=%v calls=%v", out, err, f.calls)
		}
	})
}
