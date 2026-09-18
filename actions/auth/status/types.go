package status

type Input struct {
	Environment string `json:"environment,omitempty"`
}
type Target struct {
	Environment                      string
	Default                          bool
	ServerURL                        string
	SiteContentURL                   string
	APIVersion                       string
	AuthType                         string
	PATNameVariable                  string
	PATSecretVariable                string
	StoredCredentialReferencePresent bool
	DefaultWorkspace                 string
}

type Output struct {
	Verification                     string   `json:"verification"`
	MissingVariables                 []string `json:"missing_variables,omitempty"`
	Status                           string   `json:"status"`
	Environment                      string   `json:"environment"`
	Default                          bool     `json:"default"`
	ServerURL                        string   `json:"server_url"`
	SiteContentURL                   string   `json:"site_content_url,omitempty"`
	APIVersion                       string   `json:"api_version,omitempty"`
	AuthType                         string   `json:"auth_type"`
	PATNameVariable                  string   `json:"pat_name_env"`
	PATSecretVariable                string   `json:"pat_secret_env"`
	PATNamePresent                   bool     `json:"pat_name_present"`
	PATSecretPresent                 bool     `json:"pat_secret_present"`
	StoredCredentialReferencePresent bool     `json:"stored_credential_reference_present"`
	CredentialSource                 string   `json:"credential_source"`
	DefaultWorkspace                 string   `json:"default_workspace,omitempty"`
	Help                             []string `json:"help"`
}

type CompactResult struct {
	Verification                     string   `json:"verification"`
	MissingVariables                 []string `json:"missing_variables,omitempty"`
	Status                           string   `json:"status"`
	Environment                      string   `json:"environment"`
	ServerURL                        string   `json:"server_url"`
	SiteContentURL                   string   `json:"site_content_url,omitempty"`
	PATNamePresent                   bool     `json:"pat_name_present"`
	PATSecretPresent                 bool     `json:"pat_secret_present"`
	StoredCredentialReferencePresent bool     `json:"stored_credential_reference_present"`
	CredentialSource                 string   `json:"credential_source"`
	Details                          string   `json:"details"`
	Help                             []string `json:"help"`
}

type FullResult struct {
	Verification                     string   `json:"verification"`
	MissingVariables                 []string `json:"missing_variables,omitempty"`
	Status                           string   `json:"status"`
	Environment                      string   `json:"environment"`
	Default                          bool     `json:"default"`
	ServerURL                        string   `json:"server_url"`
	SiteContentURL                   string   `json:"site_content_url,omitempty"`
	APIVersion                       string   `json:"api_version,omitempty"`
	AuthType                         string   `json:"auth_type"`
	PATNameVariable                  string   `json:"pat_name_env"`
	PATSecretVariable                string   `json:"pat_secret_env"`
	PATNamePresent                   bool     `json:"pat_name_present"`
	PATSecretPresent                 bool     `json:"pat_secret_present"`
	StoredCredentialReferencePresent bool     `json:"stored_credential_reference_present"`
	CredentialSource                 string   `json:"credential_source"`
	DefaultWorkspace                 string   `json:"default_workspace,omitempty"`
	Help                             []string `json:"help"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Verification: o.Verification, MissingVariables: o.MissingVariables, Status: o.Status, Environment: o.Environment, ServerURL: o.ServerURL, SiteContentURL: o.SiteContentURL, PATNamePresent: o.PATNamePresent, PATSecretPresent: o.PATSecretPresent, StoredCredentialReferencePresent: o.StoredCredentialReferencePresent, CredentialSource: o.CredentialSource, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Verification: o.Verification, MissingVariables: o.MissingVariables, Status: o.Status, Environment: o.Environment, Default: o.Default, ServerURL: o.ServerURL, SiteContentURL: o.SiteContentURL, APIVersion: o.APIVersion, AuthType: o.AuthType, PATNameVariable: o.PATNameVariable, PATSecretVariable: o.PATSecretVariable, PATNamePresent: o.PATNamePresent, PATSecretPresent: o.PATSecretPresent, StoredCredentialReferencePresent: o.StoredCredentialReferencePresent, CredentialSource: o.CredentialSource, DefaultWorkspace: o.DefaultWorkspace, Help: o.Help}
}
