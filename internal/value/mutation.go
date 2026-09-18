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
