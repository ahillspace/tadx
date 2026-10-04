package auth

import "strings"

// LocalPATReadiness contains non-secret process and stored-reference facts.
type LocalPATReadiness struct {
	NamePresent   bool
	SecretPresent bool
	Source        string
	Ready         bool
}

// InspectLocalPATReadiness checks references without opening the credential store.
func InspectLocalPATReadiness(nameVariable, secretVariable string, storedReference bool, lookup LookupEnv) LocalPATReadiness {
	name, nameExists := lookup.LookupEnv(nameVariable)
	secret, secretExists := lookup.LookupEnv(secretVariable)
	result := LocalPATReadiness{
		NamePresent:   nameExists && strings.TrimSpace(name) != "",
		SecretPresent: secretExists && strings.TrimSpace(secret) != "",
		Source:        "none",
	}
	switch {
	case result.NamePresent && result.SecretPresent:
		result.Source, result.Ready = "environment", true
	case result.NamePresent || result.SecretPresent:
		result.Source = "environment"
	case storedReference:
		result.Source, result.Ready = "os_credential_store", true
	}
	return result
}
