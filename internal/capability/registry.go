package capability

//go:generate go run ../../cmd/gencapdocs -out ../../docs/reference/capabilities.md -json-out ../../docs/reference/capabilities.json

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// All returns the canonical registry sorted by stable capability ID.
func All() []Definition {
	definitions := make([]Definition, len(canonicalDefinitions))
	batchSelectors := BatchSelectors()
	for index, definition := range canonicalDefinitions {
		definitions[index] = clone(definition)
		_, definitions[index].SupportsBatch = batchSelectors[definition.ID]
	}
	slices.SortFunc(definitions, func(left, right Definition) int { return cmp.Compare(left.ID, right.ID) })
	return definitions
}

// Lookup finds one capability by an exact, case-sensitive stable ID.
func Lookup(id string) (Definition, bool) {
	definitions := All()
	index, found := slices.BinarySearchFunc(definitions, id, func(definition Definition, target string) int {
		return cmp.Compare(definition.ID, target)
	})
	if !found {
		return Definition{}, false
	}
	return clone(definitions[index]), true
}

// Executable returns implemented local CLI capabilities with command bindings.
func Executable() []Definition {
	definitions := All()
	return slices.DeleteFunc(definitions, func(definition Definition) bool {
		return definition.Implementation != ImplementationImplemented || len(definition.CommandPath) == 0
	})
}

// Validate checks the complete registry contract.
func Validate(definitions []Definition) error {
	ids := make(map[string]struct{}, len(definitions))
	paths := make(map[string]string)
	for index, definition := range definitions {
		prefix := fmt.Sprintf("definition %d (%q)", index, definition.ID)
		if err := validateRequired(prefix, definition); err != nil {
			return err
		}
		if _, exists := ids[definition.ID]; exists {
			return fmt.Errorf("duplicate capability ID %q", definition.ID)
		}
		ids[definition.ID] = struct{}{}
		if !validOwner(definition.Owner) {
			return fmt.Errorf("%s has invalid owner %q", prefix, definition.Owner)
		}
		if definition.RemoteMutation && !definition.SupportsPreview {
			return fmt.Errorf("%s: remote mutation must support preview", prefix)
		}
		if definition.SupportsPreview && !definition.RemoteMutation && !definition.LocalWrite {
			return fmt.Errorf("%s: supports preview without consequential write", prefix)
		}
		if definition.Disposition == DispositionDelegated && len(definition.CommandPath) != 0 {
			return fmt.Errorf("%s: delegated capability has local command binding", prefix)
		}
		if definition.Disposition == DispositionDelegated && definition.Implementation != ImplementationExternalDelegated {
			return fmt.Errorf("%s: delegated capability must have external/delegated implementation", prefix)
		}
		if definition.Owner != OwnerCLI && definition.Disposition != DispositionDelegated {
			return fmt.Errorf("%s: non-CLI owner must be delegated", prefix)
		}
		if definition.Implementation == ImplementationImplemented && definition.Owner == OwnerCLI && len(definition.CommandPath) == 0 {
			return fmt.Errorf("%s: implemented CLI capability has no binding", prefix)
		}
		if definition.Implementation != ImplementationImplemented && len(definition.CommandPath) != 0 {
			return fmt.Errorf("%s: non-implemented capability has command binding", prefix)
		}
		if definition.Verification == VerificationBlocked {
			if !validBlocker(definition.Blocker) {
				if definition.Blocker == "" {
					return fmt.Errorf("%s: blocked capability has no blocker", prefix)
				}
				return fmt.Errorf("%s has invalid blocker %q", prefix, definition.Blocker)
			}
			if len(definition.CommandPath) != 0 || definition.Implementation == ImplementationImplemented {
				return fmt.Errorf("%s: blocked capability is executable", prefix)
			}
		} else if definition.Blocker != "" {
			if !validBlocker(definition.Blocker) {
				return fmt.Errorf("%s has invalid blocker %q", prefix, definition.Blocker)
			}
			return fmt.Errorf("%s: ready capability references blocker %q", prefix, definition.Blocker)
		}
		if len(definition.CommandPath) != 0 {
			path := strings.Join(definition.CommandPath, " ")
			if previous, exists := paths[path]; exists {
				return fmt.Errorf("duplicate implemented command path %q for %q and %q", path, previous, definition.ID)
			}
			paths[path] = definition.ID
		}
	}
	return nil
}

// ValidateBindings rejects command bindings that drift from registry metadata.
func ValidateBindings(definitions []Definition, bindings []Binding) error {
	if err := Validate(definitions); err != nil {
		return err
	}
	byID := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		byID[definition.ID] = definition
	}
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		definition, exists := byID[binding.CapabilityID]
		if !exists {
			return fmt.Errorf("binding has no registry entry: %q", binding.CapabilityID)
		}
		if _, exists := seen[binding.CapabilityID]; exists {
			return fmt.Errorf("duplicate binding for %q", binding.CapabilityID)
		}
		seen[binding.CapabilityID] = struct{}{}
		if definition.Implementation != ImplementationImplemented {
			return fmt.Errorf("binding references non-implemented capability: %q", binding.CapabilityID)
		}
		if len(binding.CommandPath) == 0 {
			return fmt.Errorf("binding has empty command path: %q", binding.CapabilityID)
		}
		if !slices.Equal(binding.CommandPath, definition.CommandPath) {
			return fmt.Errorf("binding path for %q does not match registry", binding.CapabilityID)
		}
	}
	for _, definition := range definitions {
		if definition.Implementation == ImplementationImplemented {
			if _, exists := seen[definition.ID]; !exists {
				return fmt.Errorf("implemented registry entry has no binding: %q", definition.ID)
			}
		}
	}
	return nil
}

func validateRequired(prefix string, definition Definition) error {
	required := []struct {
		name  string
		value string
	}{
		{"id", definition.ID}, {"surface", definition.Surface}, {"outcome", definition.Outcome},
		{"type", string(definition.Type)}, {"disposition", string(definition.Disposition)}, {"owner", string(definition.Owner)},
		{"selectors", definition.Selectors}, {"availability", definition.Availability}, {"safety_guard", definition.SafetyGuard},
		{"artifact_effect", definition.ArtifactEffect}, {"upstream_operation", definition.Upstream}, {"evidence", definition.Evidence},
		{"evidence_level", string(definition.EvidenceLevel)}, {"verification", string(definition.Verification)},
		{"implementation", string(definition.Implementation)}, {"validation", definition.Validation},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%s missing required field %q", prefix, field.name)
		}
	}
	if !slices.Contains([]OperationType{OperationFind, OperationInspect, OperationChange, OperationDeliver}, definition.Type) {
		return fmt.Errorf("%s has invalid operation type %q", prefix, definition.Type)
	}
	if !slices.Contains([]Disposition{DispositionShip, DispositionDelegated}, definition.Disposition) {
		return fmt.Errorf("%s has invalid disposition %q", prefix, definition.Disposition)
	}
	if !slices.Contains([]EvidenceLevel{EvidenceArchitectureLocked, EvidenceLocalContract, EvidenceDocsOnly, EvidenceContractVerified, EvidenceLiveVerified}, definition.EvidenceLevel) {
		return fmt.Errorf("%s has invalid evidence level %q", prefix, definition.EvidenceLevel)
	}
	if !slices.Contains([]VerificationReadiness{VerificationReady, VerificationBlocked}, definition.Verification) {
		return fmt.Errorf("%s has invalid verification readiness %q", prefix, definition.Verification)
	}
	if !slices.Contains([]ImplementationState{ImplementationPlanned, ImplementationImplemented, ImplementationExternalDelegated}, definition.Implementation) {
		return fmt.Errorf("%s has invalid implementation state %q", prefix, definition.Implementation)
	}
	return nil
}

func validOwner(owner Owner) bool {
	return slices.Contains([]Owner{OwnerCLI, OwnerMCP, OwnerTableauDesktopMCP, OwnerAgentSkill}, owner)
}

func validBlocker(blocker BlockerID) bool {
	return slices.Contains([]BlockerID{BlockerB1, BlockerB2, BlockerB3, BlockerB4, BlockerB6}, blocker)
}
