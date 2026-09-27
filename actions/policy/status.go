package policy

import (
	"github.com/ahillspace/tadx/internal/value"
)

type StatusOutput struct {
	Policy  value.ManagedPolicyStatus `json:"policy"`
	Allowed int                       `json:"allowed_capabilities"`
	Denied  int                       `json:"denied_capabilities"`
	Help    []string                  `json:"help"`
}

func (o StatusOutput) CompactOutput() any {
	return o.render(false)
}
func (o StatusOutput) FullOutput() any { return o.render(true) }

// DetailCommand keeps policy diagnostics available when last is disallowed.
func (o StatusOutput) DetailCommand() []string { return []string{"policy", "status", "--full"} }

func (o StatusOutput) render(full bool) any {
	result := struct {
		State           string                               `json:"state"`
		Path            string                               `json:"path"`
		Protected       bool                                 `json:"protected"`
		PathProtected   bool                                 `json:"path_protected"`
		Warnings        []string                             `json:"warnings,omitempty"`
		CandidateValid  bool                                 `json:"candidate_valid"`
		RemoteMutations bool                                 `json:"remote_mutations"`
		Allowed         int                                  `json:"allowed_capabilities"`
		Denied          int                                  `json:"denied_capabilities"`
		Reason          string                               `json:"reason,omitempty"`
		Details         string                               `json:"details,omitempty"`
		Help            []string                             `json:"help"`
		AllowedIDs      []string                             `json:"allowed_capability_ids,omitempty"`
		Checks          []value.ManagedPolicyProtectionCheck `json:"protection_checks,omitempty"`
	}{State: o.Policy.State, Path: o.Policy.Path, Protected: o.Policy.Protected, PathProtected: o.Policy.PathProtected, Warnings: o.Policy.Warnings, CandidateValid: o.Policy.CandidateValid, RemoteMutations: o.Policy.RemoteMutations, Allowed: o.Allowed, Denied: o.Denied, Reason: o.Policy.Reason, Details: "--full", Help: o.Help}
	if full {
		result.Details = ""
		result.AllowedIDs = o.Policy.AllowedCapabilities
		result.Checks = o.Policy.Checks
	}
	return result
}
