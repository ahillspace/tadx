package status

type Input struct {
	Environment string `json:"environment,omitempty"`
}
type Target struct {
	Environment       string
	Default           bool
	ServerURL         string
	SiteContentURL    string
	APIVersion        string
	AuthType          string
	PATNameVariable   string
	PATSecretVariable string
	DefaultWorkspace  string
}

type Output struct {
	Status            string   `json:"status"`
	Environment       string   `json:"environment"`
	Default           bool     `json:"default"`
	ServerURL         string   `json:"server_url"`
	SiteContentURL    string   `json:"site_content_url,omitempty"`
	APIVersion        string   `json:"api_version,omitempty"`
	AuthType          string   `json:"auth_type"`
	PATNameVariable   string   `json:"pat_name_env"`
	PATSecretVariable string   `json:"pat_secret_env"`
	PATNamePresent    bool     `json:"pat_name_present"`
	PATSecretPresent  bool     `json:"pat_secret_present"`
	DefaultWorkspace  string   `json:"default_workspace,omitempty"`
	Help              []string `json:"help"`
}

type CompactResult struct {
	Status           string   `json:"status"`
	Environment      string   `json:"environment"`
	ServerURL        string   `json:"server_url"`
	SiteContentURL   string   `json:"site_content_url,omitempty"`
	PATNamePresent   bool     `json:"pat_name_present"`
	PATSecretPresent bool     `json:"pat_secret_present"`
	Details          string   `json:"details"`
	Help             []string `json:"help"`
}

type FullResult struct {
	Status            string   `json:"status"`
	Environment       string   `json:"environment"`
	Default           bool     `json:"default"`
	ServerURL         string   `json:"server_url"`
	SiteContentURL    string   `json:"site_content_url,omitempty"`
	APIVersion        string   `json:"api_version,omitempty"`
	AuthType          string   `json:"auth_type"`
	PATNameVariable   string   `json:"pat_name_env"`
	PATSecretVariable string   `json:"pat_secret_env"`
	PATNamePresent    bool     `json:"pat_name_present"`
	PATSecretPresent  bool     `json:"pat_secret_present"`
	DefaultWorkspace  string   `json:"default_workspace,omitempty"`
	Help              []string `json:"help"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, ServerURL: o.ServerURL, SiteContentURL: o.SiteContentURL, PATNamePresent: o.PATNamePresent, PATSecretPresent: o.PATSecretPresent, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Default: o.Default, ServerURL: o.ServerURL, SiteContentURL: o.SiteContentURL, APIVersion: o.APIVersion, AuthType: o.AuthType, PATNameVariable: o.PATNameVariable, PATSecretVariable: o.PATSecretVariable, PATNamePresent: o.PATNamePresent, PATSecretPresent: o.PATSecretPresent, DefaultWorkspace: o.DefaultWorkspace, Help: o.Help}
}
