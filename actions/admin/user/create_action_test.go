package user_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/user"
)

type finder struct{}

func (finder) UserExists(_ context.Context, _ string) (bool, error) { return false, nil }

type creator struct{}

func (creator) CreateUser(_ context.Context, in action.CreateRequest) (action.Record, error) {
	return action.Record{LUID: "user-1", Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdPConfigurationID: in.IdPConfigurationID, IdentityPoolName: in.IdentityPoolName, Email: in.Email, Language: in.Language, Locale: in.Locale}, nil
}

func TestCreateResultCarriesProviderReturnedSettings(t *testing.T) {
	in := action.CreateInput{Environment: "dev", Site: "site", Name: "alex", SiteRole: "Creator", AuthSetting: "SAML", IdentityPoolName: "pool-1", Email: "alex@example.com", Language: "en", Locale: "en_US"}
	out, err := action.Create(t.Context(), finder{}, creator{}, in, false)
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

func (omittingCreator) CreateUser(_ context.Context, in action.CreateRequest) (action.Record, error) {
	return action.Record{LUID: "user-1", Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting}, nil
}

func TestRequestedMissingSettingsAreUnverifiedNotEchoed(t *testing.T) {
	out, err := action.Create(t.Context(), finder{}, omittingCreator{}, action.CreateInput{Environment: "dev", Site: "site", Name: "alex", SiteRole: "Viewer", AuthSetting: "SAML", Email: "requested@example.test"}, false)
	if err != nil || out.Result == nil || len(out.Result.UnverifiedSettings) != 1 || out.Result.UnverifiedSettings[0] != "email" || out.Result.User.Email != "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}
