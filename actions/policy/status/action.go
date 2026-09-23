// Package status reports the fixed machine policy and effective ceiling.
package status

import (
	"context"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type Output struct {
	Policy  value.ManagedPolicyStatus `json:"policy"`
	Allowed int                       `json:"allowed_capabilities"`
	Denied  int                       `json:"denied_capabilities"`
	Help    []string                  `json:"help"`
}
type Reader interface {
	ReadPolicy(context.Context) (Output, error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func (a *Action) Execute(ctx context.Context) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, &errs.Error{ID: "policy.status.unconfigured", Kind: errs.KindRuntime, Operation: "policy.status", Summary: "Managed policy status is not configured.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	return a.reader.ReadPolicy(ctx)
}
func (o Output) CompactOutput() any {
	return o.render(false)
}
func (o Output) FullOutput() any { return o.render(true) }

// DetailCommand keeps policy diagnostics available when last is disallowed.
func (o Output) DetailCommand() []string { return []string{"policy", "status", "--full"} }

func (o Output) render(full bool) any {
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
