package capability

import "strings"

// Discovery is the neutral, bounded contract representation shared by
// capability discovery actions.
//
// State, Blocked, and Product are compatibility inputs for list sources.
// They are derived from the contract fields for registry-backed discoveries
// and are deliberately omitted from the detailed JSON representation.
type Discovery struct {
	ID                    string   `json:"id"`
	Domain                string   `json:"domain"`
	Resource              string   `json:"resource,omitempty"`
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
	ExecutionEnabled      bool     `json:"execution_enabled"`
	SupportsPreview       bool     `json:"supports_preview"`
	SupportsBatch         bool     `json:"supports_batch"`
	LocalWrite            bool     `json:"local_write"`
	RawCapable            bool     `json:"raw_capable"`

	// Legacy list-source fields remain available to keep custom list sources
	// source-compatible while they migrate to the detailed contract shape.
	State   string `json:"-"`
	Blocked bool   `json:"-"`
	Product string `json:"-"`
}

// Summary is the bounded compact view of one capability.
type Summary struct {
	ID               string `json:"id"`
	Owner            string `json:"owner"`
	Disposition      string `json:"disposition,omitempty"`
	State            string `json:"state"`
	Command          string `json:"command,omitempty"`
	Blocked          bool   `json:"blocked"`
	ExecutionEnabled bool   `json:"execution_enabled"`
}

// Summary returns the compact representation without exposing contract-only
// fields such as selectors, safety constraints, or evidence.
func (d Discovery) Summary() Summary {
	state := d.ImplementationState
	if state == "" {
		state = d.State
	}
	blocked := d.VerificationReadiness == string(VerificationBlocked) || d.Blocked
	return Summary{
		ID:               d.ID,
		Owner:            d.Owner,
		Disposition:      d.Disposition,
		State:            state,
		Command:          d.Command,
		Blocked:          blocked,
		ExecutionEnabled: d.ExecutionEnabled,
	}
}

// FromDefinition converts one canonical registry entry to the shared
// discovery contract.
func FromDefinition(definition Definition) Discovery {
	domain, resource := classify(definition)
	verb := ""
	parts := strings.Split(definition.ID, ".")
	if len(parts) > 0 {
		verb = parts[len(parts)-1]
	}
	return Discovery{
		ID:                    definition.ID,
		Domain:                domain,
		Resource:              resource,
		Verb:                  verb,
		Surface:               definition.Surface,
		Outcome:               definition.Outcome,
		OperationType:         string(definition.Type),
		Owner:                 string(definition.Owner),
		MCPOverlap:            definition.MCPOverlap,
		Disposition:           string(definition.Disposition),
		EvidenceLevel:         string(definition.EvidenceLevel),
		VerificationReadiness: string(definition.Verification),
		ImplementationState:   string(definition.Implementation),
		Command:               strings.Join(definition.CommandPath, " "),
		Selectors:             []string{definition.Selectors},
		Availability:          definition.Availability,
		SafetyGuard:           definition.SafetyGuard,
		ArtifactEffect:        definition.ArtifactEffect,
		UpstreamOperation:     definition.Upstream,
		Evidence:              definition.Evidence,
		Validation:            definition.Validation,
		Blocker:               string(definition.Blocker),
		RemoteMutation:        definition.RemoteMutation,
		SupportsPreview:       definition.SupportsPreview,
		SupportsBatch:         definition.SupportsBatch,
		LocalWrite:            definition.LocalWrite,
		RawCapable:            definition.RawCapable,
		State:                 string(definition.Implementation),
		Blocked:               definition.Verification == VerificationBlocked,
		Product:               definition.Availability,
	}
}

// AllDiscoveries returns the canonical registry in stable ID order.
func AllDiscoveries() []Discovery {
	definitions := All()
	result := make([]Discovery, len(definitions))
	for index, definition := range definitions {
		result[index] = FromDefinition(definition)
	}
	return result
}

// LookupDiscovery returns one exact canonical registry entry as a discovery
// contract.
func LookupDiscovery(id string) (Discovery, bool) {
	definition, ok := Lookup(id)
	if !ok {
		return Discovery{}, false
	}
	return FromDefinition(definition), true
}

func classify(definition Definition) (string, string) {
	if len(definition.CommandPath) >= 3 && (definition.CommandPath[0] == "catalog" || definition.CommandPath[0] == "admin") {
		return definition.CommandPath[0], definition.CommandPath[1]
	}
	parts := strings.Split(definition.ID, ".")
	if definition.Owner == OwnerCLI && len(parts) > 0 && (parts[0] == "workbook" || parts[0] == "datasource" || parts[0] == "flow" || parts[0] == "project") {
		return "content", parts[0]
	}
	if len(parts) >= 3 {
		return parts[0], parts[1]
	}
	if len(parts) == 0 {
		return "", ""
	}
	return parts[0], ""
}
