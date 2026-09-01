package add

type Input struct {
	Alias            string `json:"alias"`
	ServerURL        string `json:"server_url"`
	SiteContentURL   string `json:"site_content_url,omitempty"`
	APIVersion       string `json:"api_version,omitempty"`
	PATNameEnv       string `json:"pat_name_env,omitempty"`
	PATSecretEnv     string `json:"pat_secret_env,omitempty"`
	DefaultWorkspace string `json:"default_workspace,omitempty"`
}

type Profile struct {
	Alias            string `json:"alias"`
	ServerURL        string `json:"server_url"`
	SiteContentURL   string `json:"site_content_url,omitempty"`
	APIVersion       string `json:"api_version,omitempty"`
	AuthType         string `json:"auth_type"`
	PATNameEnv       string `json:"pat_name_env"`
	PATSecretEnv     string `json:"pat_secret_env"`
	DefaultWorkspace string `json:"default_workspace,omitempty"`
}

type Output struct {
	Status  string   `json:"status"`
	Profile Profile  `json:"environment"`
	Help    []string `json:"help"`
}

type CompactProfile struct {
	Alias          string `json:"alias"`
	ServerURL      string `json:"server_url"`
	SiteContentURL string `json:"site_content_url,omitempty"`
}
type CompactResult struct {
	Status  string         `json:"status"`
	Profile CompactProfile `json:"environment"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}
type FullResult struct {
	Status  string   `json:"status"`
	Profile Profile  `json:"environment"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Profile: CompactProfile{Alias: o.Profile.Alias, ServerURL: o.Profile.ServerURL, SiteContentURL: o.Profile.SiteContentURL}, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Profile: o.Profile, Help: o.Help}
}
