// Package profile implements environment profile operations.
package profile

// Profile is the non-secret record shared by get and list projections.
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
