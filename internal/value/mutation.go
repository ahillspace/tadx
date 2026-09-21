package value

// MutationSetting describes consent for one resolved Tableau site.
type MutationSetting struct {
	Environment    string `json:"environment,omitempty"`
	ServerURL      string `json:"server_url,omitempty"`
	SiteContentURL string `json:"site_content_url"`
	Enabled        bool   `json:"enabled"`
	Source         string `json:"source"`
	Saved          *bool  `json:"saved_enabled,omitempty"`
	Scope          string `json:"scope"`
	SourceSetting  string `json:"source_setting,omitempty"`
	Persisted      *bool  `json:"persisted,omitempty"`
}

// MutationStatus inventories site consent without implying execution permission.
type MutationStatus struct {
	Sites       []MutationConsent `json:"sites"`
	Restriction string            `json:"restriction,omitempty"`
}

// MutationConsent identifies one configured alias and its exact site's consent.
type MutationConsent struct {
	Environment    string `json:"environment"`
	Enabled        bool   `json:"enabled"`
	ServerURL      string `json:"server_url"`
	SiteContentURL string `json:"site_content_url"`
	Source         string `json:"source"`
}

func (s MutationStatus) CompactOutput() any {
	type site struct {
		Environment string `json:"environment"`
		Enabled     bool   `json:"enabled"`
	}
	result := struct {
		Sites       []site `json:"sites"`
		Restriction string `json:"restriction,omitempty"`
	}{Sites: make([]site, len(s.Sites)), Restriction: s.Restriction}
	for i, consent := range s.Sites {
		result.Sites[i] = site{Environment: consent.Environment, Enabled: consent.Enabled}
	}
	return result
}
