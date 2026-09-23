package value

// ManagedPolicyProtectionCheck describes one machine policy protection check.
type ManagedPolicyProtectionCheck struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Passed bool   `json:"passed"`
	Reason string `json:"reason,omitempty"`
}

// ManagedPolicyStatus reports a policy snapshot without exposing its loader.
type ManagedPolicyStatus struct {
	State               string                         `json:"state"`
	Path                string                         `json:"path"`
	Reason              string                         `json:"reason,omitempty"`
	CandidateValid      bool                           `json:"candidate_valid"`
	Protected           bool                           `json:"protected"`
	PathProtected       bool                           `json:"path_protected"`
	Warnings            []string                       `json:"warnings,omitempty"`
	Checks              []ManagedPolicyProtectionCheck `json:"checks"`
	AllowedCapabilities []string                       `json:"allowed_capabilities"`
	RemoteMutations     bool                           `json:"remote_mutations"`
}
