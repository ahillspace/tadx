package config

import (
	"strings"
	"testing"
)

func TestSiteMutationIdentity(t *testing.T) {
	cfg := Config{MutationsEnabled: new(true)}
	target := Environment{URL: "https://TABLEAU.example.com:443/", SiteContentURL: "Sales"}
	if saved, err := cfg.MutationSetting(target); err != nil || saved != nil {
		t.Fatalf("legacy global enabled site: %v %v", saved, err)
	}
	if err := cfg.SetMutationSetting(target, true); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, server, site string
		enabled            bool
	}{
		{"alias", "https://tableau.example.com", "Sales", true},
		{"other site", "https://tableau.example.com", "sales", false},
		{"default site", "https://tableau.example.com", "", false},
		{"other server", "https://other.example.com", "Sales", false},
		{"other port", "https://tableau.example.com:444", "Sales", false},
		{"server path", "https://tableau.example.com/path", "Sales", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			saved, err := cfg.MutationSetting(Environment{URL: tt.server, SiteContentURL: tt.site})
			if err != nil {
				t.Fatal(err)
			}
			enabled := saved != nil && *saved
			if enabled != tt.enabled {
				t.Fatalf("enabled=%v want %v", enabled, tt.enabled)
			}
		})
	}
	if err := cfg.SetMutationSetting(Environment{URL: "https://tableau.example.com", SiteContentURL: "Sales"}, false); err != nil {
		t.Fatal(err)
	}
	if len(cfg.SiteMutations) != 1 || cfg.SiteMutations[0].Enabled {
		t.Fatalf("alias did not update same site: %#v", cfg.SiteMutations)
	}
}

func TestSiteMutationDuplicateCanonicalIdentityRejected(t *testing.T) {
	cfg := Config{Version: CurrentVersion, SiteMutations: []SiteMutation{
		{ServerURL: "https://tableau.example.com", SiteContentURL: "", Enabled: true},
		{ServerURL: "https://TABLEAU.example.com:443/", SiteContentURL: "", Enabled: false},
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("conflicting canonical identities accepted")
	}
}

func TestSiteMutationValidatesWholeListBeforeMatching(t *testing.T) {
	target := Environment{URL: "https://tableau.example.com", SiteContentURL: "Sales"}
	for _, tt := range []struct {
		name, laterServer, laterSite, wantError string
	}{
		{"invalid later server", "://invalid", "Other", "site mutation server:"},
		{"duplicate later identity", "https://TABLEAU.example.com:443/", "Sales", "duplicate site mutation setting"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{SiteMutations: []SiteMutation{
				{ServerURL: target.URL, SiteContentURL: target.SiteContentURL, Enabled: true},
				{ServerURL: tt.laterServer, SiteContentURL: tt.laterSite, Enabled: false},
			}}
			if got, err := cfg.MutationSetting(target); got != nil || err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("read got=%v err=%v, want %q", got, err, tt.wantError)
			}
			before := append([]SiteMutation(nil), cfg.SiteMutations...)
			if err := cfg.SetMutationSetting(target, false); err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("write err=%v, want %q", err, tt.wantError)
			}
			for i, setting := range cfg.SiteMutations {
				if setting != before[i] {
					t.Fatalf("write changed setting %d after validation error", i)
				}
			}
		})
	}
}

func TestSiteMutationCloneDoesNotMutateOriginal(t *testing.T) {
	cfg := Config{SiteMutations: []SiteMutation{{ServerURL: "https://tableau.example.com", Enabled: true}}}
	clone := cloneConfig(cfg)
	if err := clone.SetMutationSetting(Environment{URL: "https://tableau.example.com"}, false); err != nil {
		t.Fatal(err)
	}
	if !cfg.SiteMutations[0].Enabled {
		t.Fatal("transaction clone changed original consent")
	}
}
