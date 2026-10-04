package mutation_test

import (
	"errors"
	"path/filepath"
	"testing"

	mutation "github.com/ahillspace/tadx/actions/mutation"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

func mutationConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev":   {URL: "https://TABLEAU.example.com:443/", SiteContentURL: "shared", Auth: config.Auth{Type: config.AuthTypePAT}},
		"alias": {URL: "https://tableau.example.com", SiteContentURL: "shared", Auth: config.Auth{Type: config.AuthTypePAT}},
		"other": {URL: "https://tableau.example.com", SiteContentURL: "other", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServicePersistsOnlyCanonicalSiteConsent(t *testing.T) {
	path := mutationConfig(t)
	service := mutation.New(func() string { return path }, nil)
	if _, err := service.WriteMutationSetting(t.Context(), "", true); err == nil {
		t.Fatal("ambiguous environment accepted for consent write")
	}
	state, err := service.WriteMutationSetting(t.Context(), "dev", true)
	if err != nil || state.Persisted == nil || !*state.Persisted || !state.Enabled || state.ServerURL != "https://tableau.example.com" || state.SiteContentURL != "shared" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
	alias, err := service.ReadMutationSetting(t.Context(), "alias")
	if err != nil || !alias.Enabled || alias.Source != "saved_site_setting" || alias.SourceSetting != "site_mutations" {
		t.Fatalf("alias=%#v err=%v", alias, err)
	}
	other, err := service.ReadMutationSetting(t.Context(), "other")
	if err != nil || other.Enabled || other.Source != "default_disabled" || other.Saved != nil {
		t.Fatalf("other=%#v err=%v", other, err)
	}
	cfg, err := config.Load(path)
	if err != nil || len(cfg.SiteMutations) != 1 || cfg.MutationsEnabled != nil {
		t.Fatalf("configuration=%#v err=%v", cfg.SiteMutations, err)
	}
}

func TestServiceStatusReportsConsentAndRestrictionSeparately(t *testing.T) {
	path := mutationConfig(t)
	service := mutation.New(func() string { return path }, func() bool { return true })
	status, err := service.ReadMutationStatus(t.Context(), "")
	if err != nil || len(status.Sites) != 3 || status.Sites[0].Environment != "alias" || status.Sites[1].Environment != "dev" || status.Sites[2].Environment != "other" || status.Restriction == "" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
	if _, err := service.WriteMutationSetting(t.Context(), "dev", true); err != nil {
		t.Fatal(err)
	}
	status, err = service.ReadMutationStatus(t.Context(), "dev")
	if err != nil || len(status.Sites) != 1 || !status.Sites[0].Enabled || status.Restriction == "" {
		t.Fatalf("status=%#v err=%v", status, err)
	}
}

func TestServicePolicyRequiresSelectedSite(t *testing.T) {
	path := mutationConfig(t)
	service := mutation.New(func() string { return path }, nil)
	enabled, source, err := service.Policy("")
	if err != nil || enabled || source != "site_selection_required" {
		t.Fatalf("enabled=%v source=%q err=%v", enabled, source, err)
	}
	if _, err := service.WriteMutationSetting(t.Context(), "dev", true); err != nil {
		t.Fatal(err)
	}
	enabled, source, err = service.Policy("alias")
	if err != nil || !enabled || source != "saved_site_setting" {
		t.Fatalf("enabled=%v source=%q err=%v", enabled, source, err)
	}
	_, source, err = mutation.New(func() string { return filepath.Join(t.TempDir(), "missing.yaml") }, nil).Policy("")
	if err != nil || source != "site_selection_required" {
		t.Fatalf("source=%q err=%v", source, err)
	}
	_, err = service.ReadMutationSetting(t.Context(), "missing")
	structured, ok := errors.AsType[*errs.Error](err)
	if !ok || structured.ID != "environment.resolve" {
		t.Fatalf("missing environment error=%v", err)
	}
}
