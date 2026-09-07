package logout

const CredentialSourceOS = "os_credential_store"

// Input selects one explicit environment credential.
type Input struct {
	Environment string
}

// Target contains nonsecret credential storage identity.
type Target struct {
	Environment string
}

// RemoveResult reports whether a stored credential existed.
type RemoveResult struct {
	Removed bool
}

// Output reports local credential removal and remote PAT status separately.
type Output struct {
	Status            string   `json:"status"`
	Environment       string   `json:"environment"`
	CredentialSource  string   `json:"credential_source"`
	TableauPATRevoked bool     `json:"tableau_pat_revoked"`
	Help              []string `json:"help"`
}
