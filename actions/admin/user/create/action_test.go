package create_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/user/create"
)

type finder struct{}

func (finder) FindUsers(_ context.Context, _ string) ([]action.User, error) { return nil, nil }

type creator struct{}

func (creator) CreateUser(_ context.Context, in action.Request) (action.User, error) {
	return action.User{LUID: "user-1", Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdPConfigurationID: in.IdPConfigurationID, IdentityPoolName: in.IdentityPoolName, Email: in.Email, Language: in.Language, Locale: in.Locale}, nil
}

func TestCreateResultCarriesProviderReturnedSettings(t *testing.T) {
	in := action.Input{Environment: "dev", Site: "site", Name: "alex", SiteRole: "Creator", AuthSetting: "SAML", IdentityPoolName: "pool-1", Email: "alex@example.com", Language: "en", Locale: "en_US"}
	out, err := action.New(finder{}, creator{}).Execute(t.Context(), in, false)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result == nil {
		t.Fatal("create result is nil")
	}
	got := out.Result.User
	if got.IdentityPoolName != in.IdentityPoolName || got.Email != in.Email || got.Language != in.Language || got.Locale != in.Locale {
		t.Fatalf("returned settings = %+v, want identity pool=%q email=%q language=%q locale=%q", got, in.IdentityPoolName, in.Email, in.Language, in.Locale)
	}
	compact, _ := json.Marshal(out.CompactOutput())
	if !strings.Contains(string(compact), `"result":{"status":"created","user_luid":"user-1","site_role":"Creator"`) {
		t.Fatalf("missing compact observed settings: %s", compact)
	}
}

type omittingCreator struct{}

func (omittingCreator) CreateUser(_ context.Context, in action.Request) (action.User, error) {
	return action.User{LUID: "user-1", Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting}, nil
}

func TestRequestedMissingSettingsAreUnverifiedNotEchoed(t *testing.T) {
	out, err := action.New(finder{}, omittingCreator{}).Execute(t.Context(), action.Input{Environment: "dev", Site: "site", Name: "alex", SiteRole: "Viewer", AuthSetting: "SAML", Email: "requested@example.test"}, false)
	if err != nil || out.Result == nil || len(out.Result.UnverifiedSettings) != 1 || out.Result.UnverifiedSettings[0] != "email" || out.Result.User.Email != "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
