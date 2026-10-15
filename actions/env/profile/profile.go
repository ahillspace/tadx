// Package profile implements environment profile operations.
package profile

// Profile is the non-secret record shared by get and list projections.
type Profile struct {
	Alias               string   `json:"alias"`
	Default             bool     `json:"default"`
	Status              string   `json:"status,omitempty"`
	Violations          []string `json:"violations,omitempty"`
	ServerURL           string   `json:"server_url,omitempty"`
	SiteContentURL      string   `json:"site_content_url,omitempty"`
	AuthType            string   `json:"auth_type,omitempty"`
	PATNameEnv          string   `json:"pat_name_env,omitempty"`
	PATSecretEnv        string   `json:"pat_secret_env,omitempty"`
	DefaultWorkspace    string   `json:"default_workspace,omitempty"`
	CacheMaxConcurrency int      `json:"cache_max_concurrency,omitempty"`
}
