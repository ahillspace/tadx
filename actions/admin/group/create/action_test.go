package create_test

import (
	"context"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/group/create"
)

type finder struct{}

func (finder) FindGroups(context.Context, string) ([]action.Group, error) { return nil, nil }

type creator struct{}

func (creator) CreateGroup(_ context.Context, in action.Request) (action.Group, error) {
	return action.Group{LUID: "group-1", Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled}, nil
}

func TestCreateResultCarriesProviderReturnedSettings(t *testing.T) {
	external := true
	in := action.Input{Environment: "dev", Site: "site", Name: "Authors", MinimumSiteRole: "Viewer", ExternalUserEnabled: &external}
	out, err := action.New(finder{}, creator{}).Execute(t.Context(), in, false)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result == nil || out.Result.MinimumSiteRole != in.MinimumSiteRole || out.Result.ExternalUserEnabled == nil || *out.Result.ExternalUserEnabled != external {
		t.Fatalf("create result = %+v, want returned group settings", out.Result)
	}
}

type omittingCreator struct{}

func (omittingCreator) CreateGroup(_ context.Context, in action.Request) (action.Group, error) {
	return action.Group{LUID: "group-1", Name: in.Name}, nil
}

func TestRequestedMissingExternalSettingIsUnverified(t *testing.T) {
	out, err := action.New(finder{}, omittingCreator{}).Execute(t.Context(), action.Input{Environment: "dev", Site: "site", Name: "Readers", ExternalUserEnabled: new(false)}, false)
	if err != nil || out.Result == nil || out.Result.ExternalUserEnabled != nil || len(out.Result.UnverifiedSettings) != 1 || out.Result.UnverifiedSettings[0] != "external_user_enabled" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
