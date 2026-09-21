package update

type StringField struct {
	Set   bool   `json:"set"`
	Value string `json:"value"`
}

type IntField struct {
	Set   bool `json:"set"`
	Value int  `json:"value"`
}

type Patch struct {
	ServerURL           StringField `json:"server_url"`
	SiteContentURL      StringField `json:"site_content_url"`
	APIVersion          StringField `json:"api_version"`
	PATNameEnv          StringField `json:"pat_name_env"`
	PATSecretEnv        StringField `json:"pat_secret_env"`
	DefaultWorkspace    StringField `json:"default_workspace"`
	CacheMaxConcurrency IntField    `json:"cache_max_concurrency"`
}

func (p Patch) Any() bool {
	return p.ServerURL.Set || p.SiteContentURL.Set || p.APIVersion.Set || p.PATNameEnv.Set || p.PATSecretEnv.Set || p.DefaultWorkspace.Set || p.CacheMaxConcurrency.Set
}

type Input struct {
	Preview bool   `json:"preview,omitempty"`
	Alias   string `json:"alias"`
	Patch   Patch  `json:"patch"`
}

type Profile struct {
	Alias               string `json:"alias"`
	Default             bool   `json:"default"`
	ServerURL           string `json:"server_url"`
	SiteContentURL      string `json:"site_content_url"`
	APIVersion          string `json:"api_version"`
	AuthType            string `json:"auth_type"`
	PATNameEnv          string `json:"pat_name_env"`
	PATSecretEnv        string `json:"pat_secret_env"`
	DefaultWorkspace    string `json:"default_workspace"`
	CacheMaxConcurrency int    `json:"cache_max_concurrency"`
}

type UpdateResult struct {
	Profile       Profile
	ChangedFields []string
}

type Output struct {
	Status        string   `json:"status"`
	Profile       Profile  `json:"environment"`
	ChangedFields []string `json:"changed_fields,omitempty"`
	Help          []string `json:"help"`
}

type CompactResult struct {
	Status        string         `json:"status"`
	Environment   map[string]any `json:"environment"`
	ChangedFields []string       `json:"changed_fields,omitempty"`
	Details       string         `json:"details"`
	Help          []string       `json:"help"`
}
type FullResult struct {
	Status        string   `json:"status"`
	Profile       Profile  `json:"environment"`
	ChangedFields []string `json:"changed_fields,omitempty"`
	Help          []string `json:"help"`
}

func (o Output) CompactOutput() any {
	profile := map[string]any{"alias": o.Profile.Alias, "server_url": o.Profile.ServerURL, "site_content_url": o.Profile.SiteContentURL}
	values := map[string]any{
		"api_version":           o.Profile.APIVersion,
		"pat_name_env":          o.Profile.PATNameEnv,
		"pat_secret_env":        o.Profile.PATSecretEnv,
		"default_workspace":     o.Profile.DefaultWorkspace,
		"cache_max_concurrency": o.Profile.CacheMaxConcurrency,
	}
	for _, field := range o.ChangedFields {
		if value, ok := values[field]; ok {
			profile[field] = value
		}
	}
	return CompactResult{Status: o.Status, Environment: profile, ChangedFields: o.ChangedFields, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Profile: o.Profile, ChangedFields: o.ChangedFields, Help: o.Help}
}
