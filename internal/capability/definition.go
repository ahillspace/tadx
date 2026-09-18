// Package capability provides the executable source of truth for TADX capability
// discovery, ownership, safety, readiness, and command binding metadata.
package capability

import "slices"

type OperationType string

const (
	OperationFind    OperationType = "find"
	OperationInspect OperationType = "inspect"
	OperationChange  OperationType = "change"
	OperationDeliver OperationType = "deliver"
)

type Disposition string

const (
	DispositionShip      Disposition = "ship"
	DispositionDelegated Disposition = "delegated"
)

type EvidenceLevel string

const (
	EvidenceArchitectureLocked EvidenceLevel = "architecture-locked"
	EvidenceLocalContract      EvidenceLevel = "local-contract"
	EvidenceDocsOnly           EvidenceLevel = "docs-only"
	EvidenceContractVerified   EvidenceLevel = "contract-verified"
	EvidenceLiveVerified       EvidenceLevel = "live-verified"
)

type VerificationReadiness string

const (
	VerificationReady   VerificationReadiness = "ready"
	VerificationBlocked VerificationReadiness = "blocked"
)

type ImplementationState string

const (
	ImplementationPlanned           ImplementationState = "planned"
	ImplementationImplemented       ImplementationState = "implemented"
	ImplementationExternalDelegated ImplementationState = "external/delegated"
)

type Owner string

const (
	OwnerCLI               Owner = "cli"
	OwnerMCP               Owner = "tableau-mcp"
	OwnerTableauDesktopMCP Owner = "tableau/desktop-mcp"
	OwnerAgentSkill        Owner = "agent/skill"
)

type BlockerID string

const (
	BlockerB1 BlockerID = "B1"
	BlockerB2 BlockerID = "B2"
	BlockerB3 BlockerID = "B3"
	BlockerB4 BlockerID = "B4"
	BlockerB6 BlockerID = "B6"
)

// Definition is one public CLI operation or delegated discoverable operation.
// CommandPath is metadata for composition-root wiring, not a Cobra factory.
type Definition struct {
	ID              string                `json:"id"`
	Surface         string                `json:"surface"`
	Outcome         string                `json:"outcome"`
	Type            OperationType         `json:"type"`
	Disposition     Disposition           `json:"disposition"`
	Owner           Owner                 `json:"owner"`
	MCPOverlap      string                `json:"mcp_overlap,omitempty"`
	Selectors       string                `json:"selectors"`
	Availability    string                `json:"availability"`
	LocalWrite      bool                  `json:"local_write"`
	RemoteMutation  bool                  `json:"remote_mutation"`
	Administrative  bool                  `json:"administrative"`
	SupportsPreview bool                  `json:"supports_preview"`
	SupportsBatch   bool                  `json:"supports_batch"`
	SafetyGuard     string                `json:"safety_guard"`
	ArtifactEffect  string                `json:"artifact_effect"`
	Upstream        string                `json:"upstream_operation"`
	Evidence        string                `json:"evidence"`
	EvidenceLevel   EvidenceLevel         `json:"evidence_level"`
	Verification    VerificationReadiness `json:"verification"`
	Implementation  ImplementationState   `json:"implementation"`
	Validation      string                `json:"validation"`
	Blocker         BlockerID             `json:"blocker,omitempty"`
	CommandPath     []string              `json:"command_path,omitempty"`
	RawCapable      bool                  `json:"raw_capable"`
}

// Binding associates composition-root command wiring with a registry ID.
type Binding struct {
	CapabilityID string   `json:"capability_id"`
	CommandPath  []string `json:"command_path"`
}

func clone(definition Definition) Definition {
	definition.CommandPath = slices.Clone(definition.CommandPath)
	return definition
}
