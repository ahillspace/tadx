package value

// MutationSetting distinguishes saved policy from the effective process override.
type MutationSetting struct {
	Enabled       bool   `json:"enabled"`
	Source        string `json:"source"`
	Saved         *bool  `json:"saved_enabled,omitempty"`
	Scope         string `json:"scope"`
	SourceSetting string `json:"source_setting,omitempty"`
	Persisted     *bool  `json:"persisted,omitempty"`
}
