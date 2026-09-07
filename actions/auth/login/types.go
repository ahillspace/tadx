package login

import (
	"encoding/json"
	"fmt"
)

const CredentialSourceOS = "os_credential_store"

// Input contains one explicit environment and the credentials read by CLI plumbing.
type Input struct {
	Environment string
	PATName     string
	PATSecret   string
}

func (i Input) String() string {
	return fmt.Sprintf("PAT login for environment %q ([REDACTED])", i.Environment)
}
func (i Input) GoString() string { return i.String() }
func (i Input) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Environment string `json:"environment"`
	}{Environment: i.Environment})
}

// Target contains nonsecret authentication and storage context.
type Target struct {
	Environment    string
	ServerURL      string
	SiteContentURL string
	APIVersion     string
}

// Credential carries PAT values only between bounded authentication dependencies.
type Credential struct {
	PATName   string
	PATSecret string
}

func (Credential) String() string     { return "PAT credential ([REDACTED])" }
func (c Credential) GoString() string { return c.String() }
func (Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Redacted bool `json:"redacted"`
	}{Redacted: true})
}

// Authentication is the authoritative identity returned by Tableau validation.
type Authentication struct {
	SiteLUID string
	UserLUID string
}

// StoreResult reports nonsecret state observed while saving the credential.
type StoreResult struct {
	EnvironmentVariablesOverride bool
}

// Output reports credential persistence without exposing PAT values.
type Output struct {
	Status           string   `json:"status"`
	Environment      string   `json:"environment"`
	CredentialSource string   `json:"credential_source"`
	Validated        bool     `json:"validated"`
	SiteLUID         string   `json:"site_luid,omitempty"`
	UserLUID         string   `json:"user_luid,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	Help             []string `json:"help"`
}

// CompactResult omits identity details available through --full.
type CompactResult struct {
	Status           string   `json:"status"`
	Environment      string   `json:"environment"`
	CredentialSource string   `json:"credential_source"`
	Validated        bool     `json:"validated"`
	Warnings         []string `json:"warnings,omitempty"`
	Details          string   `json:"details"`
	Help             []string `json:"help"`
}

// CompactOutput returns the bounded default result.
func (o Output) CompactOutput() any {
	return CompactResult{
		Status: o.Status, Environment: o.Environment, CredentialSource: o.CredentialSource,
		Validated: o.Validated, Warnings: o.Warnings, Details: "--full", Help: o.Help,
	}
}

// FullOutput returns the validated Tableau identity without credential values.
func (o Output) FullOutput() any { return o }
