package logout

const CredentialSourceOS = "os_credential_store"

// Input selects one explicit environment credential.
type Input struct {
	Environment string
	Preview     bool
}

// Target contains nonsecret credential storage identity.
type Target struct {
	Environment                      string
	EnvironmentCredentialsAvailable  bool
	StoredCredentialReferencePresent bool
}

// RemoveResult reports whether a stored credential existed.
type RemoveResult struct {
	Removed bool
}

// Output reports local credential removal and remote PAT status separately.
type Output struct {
	Plan              *Plan    `json:"plan,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	Status            string   `json:"status"`
	Environment       string   `json:"environment"`
	CredentialSource  string   `json:"credential_source"`
	TableauPATRevoked bool     `json:"tableau_pat_revoked"`
	Help              []string `json:"help"`
}

// Plan reports configured local removal scope without opening stored credentials.
type Plan struct {
	StoredCredentialReferencePresent bool `json:"stored_credential_reference_present"`
	EnvironmentCredentialsAvailable  bool `json:"environment_credentials_available"`
}
