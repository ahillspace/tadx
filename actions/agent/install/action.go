// Package install implements agent.install.
package install

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/errs"
)

// Input selects a supported agent's global skill directory.
type Input struct {
	Target  string
	Preview bool
	Force   bool
}

// Skill describes one bundled package with a home-relative destination.
type Skill struct {
	Target string `json:"target,omitempty"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Files  int    `json:"files"`
	Backup string `json:"backup,omitempty"`
}

// Result contains bounded installation details.
type Result struct {
	Targets  []string
	Status   string
	Skills   []Skill
	Warnings []string
}

// Output is the installation result or read-only plan.
type Output struct {
	Targets  []string `json:"targets,omitempty"`
	Status   string   `json:"status"`
	Target   string   `json:"target"`
	Skills   []Skill  `json:"skills"`
	Warnings []string `json:"warnings,omitempty"`
	Help     []string `json:"help"`
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
func (o Output) CompactOutput() any {
	skills := make([]compactSkill, len(o.Skills))
	for i, skill := range o.Skills {
		skills[i] = compactSkill{Target: skill.Target, Name: skill.Name, Status: skill.Status, Path: skill.Path, Backup: skill.Backup}
	}
	return compactOutput{Targets: o.Targets, Status: o.Status, Target: o.Target, Skills: skills, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput includes home-relative package paths and integrity digests.
func (o Output) FullOutput() any { return o }

// Installer installs the two bundled packages without remote requests.
type Installer interface {
	Install(context.Context, Input) (Result, error)
}

// Action owns the agent.install operation.
type Action struct{ installer Installer }

// New constructs the installation action.
func New(installer Installer) *Action { return &Action{installer: installer} }

// Execute validates and installs, or previews without filesystem writes.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if input.Target != "auto" && !agenttarget.IsSupported(input.Target) {
		return Output{}, &errs.Error{ID: "agent.install.usage", Kind: errs.KindUsage, Operation: "agent.install", Summary: fmt.Sprintf("--target must be auto or %s.", agenttarget.Summary()), Retryable: errs.Bool(false), CorrectiveAction: "Choose one supported target, for example: tadx agent install --target opencode."}
	}
	if a == nil || a.installer == nil {
		return Output{}, &errs.Error{ID: "agent.install.runtime", Kind: errs.KindRuntime, Operation: "agent.install", Summary: "Agent skill installation is not configured.", Retryable: errs.Bool(false)}
	}
	result, err := a.installer.Install(ctx, input)
	output := Output{Targets: result.Targets, Status: result.Status, Target: input.Target, Skills: result.Skills, Warnings: result.Warnings, Help: []string{"tadx capability list", "tadx capability get agent.install --full"}}
	if err != nil {
		return output, &errs.Error{ID: "agent.install.failed", Kind: errs.KindOperation, Operation: "agent.install", Summary: "Agent skill installation failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the reported target and operating-system cause. Close programs holding skill files open and check directory permissions; use --preview before retrying."}
	}
	return output, nil
}
