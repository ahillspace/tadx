// Package agent owns bundled skill installation and removal.
package agent

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

// InstallInput selects a supported agent's global skill directory.
type InstallInput struct {
	Target  string
	Preview bool
	Force   bool
}

// InstallSkill describes one bundled package with a home-relative destination.
type InstallSkill struct {
	Target string `json:"target,omitempty"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Files  int    `json:"files"`
	Backup string `json:"backup,omitempty"`
}

// InstallOutput is the installation result or read-only plan.
type InstallOutput struct {
	Targets  []string       `json:"targets,omitempty"`
	Status   string         `json:"status"`
	Target   string         `json:"target"`
	Skills   []InstallSkill `json:"skills"`
	Warnings []string       `json:"warnings,omitempty"`
	Help     []string       `json:"help"`
}

type compactSkill struct {
	Target string `json:"target,omitempty"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Path   string `json:"path"`
	Backup string `json:"backup,omitempty"`
}

type compactOutput struct {
	Targets  []string       `json:"targets,omitempty"`
	Status   string         `json:"status"`
	Target   string         `json:"target"`
	Skills   []compactSkill `json:"skills"`
	Warnings []string       `json:"warnings,omitempty"`
	Details  string         `json:"details"`
	Help     []string       `json:"help"`
}

// CompactOutput returns the bounded next-decision fields.
func (o InstallOutput) CompactOutput() any {
	skills := make([]compactSkill, len(o.Skills))
	for i, skill := range o.Skills {
		skills[i] = compactSkill{Target: skill.Target, Name: skill.Name, Status: skill.Status, Path: skill.Path, Backup: skill.Backup}
	}
	return compactOutput{Targets: o.Targets, Status: o.Status, Target: o.Target, Skills: skills, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput includes home-relative package paths and integrity digests.
func (o InstallOutput) FullOutput() any { return o }

// GuidanceInstaller provides the native skill mutations.
type GuidanceInstaller interface {
	Install(context.Context, string, bool, bool) (value.AgentGuidanceResult, error)
	Uninstall(context.Context, string, bool, bool) (value.AgentGuidanceResult, error)
}

// Service owns the install and uninstall workflows.
type Service struct{ installer GuidanceInstaller }

// New constructs the agent service.
func New(installer GuidanceInstaller) *Service { return &Service{installer: installer} }

// Install validates and installs, or previews without filesystem writes.
func (a *Service) Install(ctx context.Context, input InstallInput) (InstallOutput, error) {
	if input.Target != "auto" && !agenttarget.IsSupported(input.Target) {
		return InstallOutput{}, &errs.Error{ID: "agent.install.usage", Kind: errs.KindUsage, Operation: "agent.install", Summary: fmt.Sprintf("--target must be auto or %s.", agenttarget.Summary()), Retryable: errs.Bool(false), CorrectiveAction: "Choose one supported target, for example: tadx agent install --target opencode."}
	}
	result, err := a.installer.Install(ctx, input.Target, input.Preview, input.Force)
	var failure error
	if err != nil {
		failure = &errs.Error{ID: "agent.install.failed", Kind: errs.KindOperation, Operation: "agent.install", Summary: "Agent skill installation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the reported target and operating-system cause. Close programs holding skill files open and check directory permissions; use --preview before retrying."}
		// Only packages the installer reports as changed or failed accompany the error.
		if len(result.Skills) == 0 {
			return InstallOutput{}, failure
		}
	}
	skills := make([]InstallSkill, len(result.Skills))
	for index, skill := range result.Skills {
		skills[index] = InstallSkill{Target: skill.Target, Name: skill.Name, Status: skill.Status, Path: skill.Path, SHA256: skill.SHA256, Files: skill.Files, Backup: skill.Backup}
	}
	return InstallOutput{Targets: result.Targets, Status: result.Status, Target: input.Target, Skills: skills, Warnings: result.Warnings, Help: []string{"tadx capability list", "tadx capability get agent.install --full"}}, failure
}
