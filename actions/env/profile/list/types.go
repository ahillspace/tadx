package list

import "github.com/ahillspace/tadx/internal/output"

type Input struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type Profile struct {
	Alias               string `json:"alias"`
	Default             bool   `json:"default"`
	ServerURL           string `json:"server_url"`
	SiteContentURL      string `json:"site_content_url,omitempty"`
	APIVersion          string `json:"api_version,omitempty"`
	AuthType            string `json:"auth_type"`
	PATNameEnv          string `json:"pat_name_env"`
	PATSecretEnv        string `json:"pat_secret_env"`
	DefaultWorkspace    string `json:"default_workspace,omitempty"`
	CacheMaxConcurrency int    `json:"cache_max_concurrency,omitempty"`
}

type Page = output.Page

type Output struct {
	Page     Page      `json:"page"`
	Profiles []Profile `json:"environments"`
	Help     []string  `json:"help"`
}

type CompactProfile struct {
	Alias   string `json:"alias"`
	Default bool   `json:"default"`
}

type CompactResult struct {
	Page     Page             `json:"page"`
	Profiles []CompactProfile `json:"environments"`
	Details  string           `json:"details"`
	Help     []string         `json:"help"`
}

type FullResult struct {
	Page     Page      `json:"page"`
	Profiles []Profile `json:"environments"`
	Help     []string  `json:"help"`
}

func (o Output) CompactOutput() any {
	profiles := make([]CompactProfile, len(o.Profiles))
	for index, profile := range o.Profiles {
		profiles[index] = CompactProfile{Alias: profile.Alias, Default: profile.Default}
	}
	return CompactResult{Page: o.Page, Profiles: profiles, Details: "--full", Help: o.Help}
}

func (o Output) FullOutput() any {
	return FullResult{Page: o.Page, Profiles: o.Profiles, Help: o.Help}
}
