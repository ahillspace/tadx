package get

type Input struct {
	Alias string `json:"alias"`
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

type Output struct {
	Profile Profile  `json:"environment"`
	Help    []string `json:"help"`
}

type CompactProfile = Profile

type CompactResult struct {
	Profile CompactProfile `json:"environment"`
	Details string         `json:"details"`
	Help    []string       `json:"help"`
}

type FullResult struct {
	Profile Profile  `json:"environment"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Profile: o.Profile, Details: "--full", Help: o.Help}
}

func (o Output) FullOutput() any { return FullResult{Profile: o.Profile, Help: o.Help} }
