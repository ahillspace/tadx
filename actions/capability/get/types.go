package get

// Input selects one capability by its exact registry ID.
type Input struct {
	ID string `json:"id"`
}

// Capability is the detailed discovery view of one registry entry.
type Capability struct {
	ID                    string   `json:"id"`
	Domain                string   `json:"domain"`
	Verb                  string   `json:"verb"`
	Surface               string   `json:"surface"`
	Outcome               string   `json:"outcome"`
	OperationType         string   `json:"operation_type"`
	Owner                 string   `json:"owner"`
	MCPOverlap            string   `json:"mcp_overlap,omitempty"`
	Disposition           string   `json:"disposition"`
	EvidenceLevel         string   `json:"evidence_level"`
	VerificationReadiness string   `json:"verification_readiness"`
	ImplementationState   string   `json:"implementation_state"`
	Command               string   `json:"command,omitempty"`
	Selectors             []string `json:"selectors"`
	Availability          string   `json:"availability"`
	SafetyGuard           string   `json:"safety_guard"`
	ArtifactEffect        string   `json:"artifact_effect"`
	UpstreamOperation     string   `json:"upstream_operation"`
	Evidence              string   `json:"evidence"`
	Validation            string   `json:"validation"`
	Blocker               string   `json:"blocker,omitempty"`
	RemoteMutation        bool     `json:"remote_mutation"`
	RequiresApply         bool     `json:"requires_apply"`
	LocalWrite            bool     `json:"local_write"`
	RawCapable            bool     `json:"raw_capable"`
}

// Output is the stable capability detail result.
type Output struct {
	Capability Capability `json:"capability"`
	Help       []string   `json:"help"`
}
