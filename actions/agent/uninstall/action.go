// Package uninstall implements agent.uninstall.
package uninstall

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type Input struct {
	Target  string
	Preview bool
	Force   bool
}
type Skill struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Backup string `json:"backup,omitempty"`
}
type Output struct {
	Status   string   `json:"status"`
	Target   string   `json:"target"`
	Skills   []Skill  `json:"skills"`
	Warnings []string `json:"warnings,omitempty"`
	Help     []string `json:"help"`
}

func (o Output) CompactOutput() any {
	items := make([]struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Path   string `json:"path"`
		Backup string `json:"backup,omitempty"`
	}, len(o.Skills))
	for i, v := range o.Skills {
		items[i].Name, items[i].Status, items[i].Path, items[i].Backup = v.Name, v.Status, v.Path, v.Backup
	}
	return struct {
		Status   string   `json:"status"`
		Target   string   `json:"target"`
		Skills   any      `json:"skills"`
		Warnings []string `json:"warnings,omitempty"`
		Details  string   `json:"details"`
		Help     []string `json:"help"`
	}{o.Status, o.Target, items, o.Warnings, "--full", o.Help}
}
func (o Output) FullOutput() any { return o }

type Uninstaller interface {
	Uninstall(context.Context, string, bool, bool) (value.AgentGuidanceResult, error)
}
type Action struct{ uninstaller Uninstaller }

func New(uninstaller Uninstaller) *Action { return &Action{uninstaller: uninstaller} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if !agenttarget.IsSupported(in.Target) {
		return Output{}, &errs.Error{ID: "agent.uninstall.usage", Kind: errs.KindUsage, Operation: "agent.uninstall", Summary: fmt.Sprintf("--target must be %s.", agenttarget.Summary()), Retryable: errs.Bool(false), CorrectiveAction: "Choose one supported target, for example: tadx agent uninstall --target opencode --preview."}
	}
	result, err := a.uninstaller.Uninstall(ctx, in.Target, in.Preview, in.Force)
	var failure error
	if err != nil {
		failure = &errs.Error{ID: "agent.uninstall.failed", Kind: errs.KindOperation, Operation: "agent.uninstall", Summary: "Agent Guidance uninstall failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the target directory and permissions. Use --preview first; edited TADX-owned packages are retained as backups."}
		// Only package state the uninstaller confirms after an incomplete rollback accompanies the error.
		if len(result.Skills) == 0 {
			return Output{}, failure
		}
	}
	skills := make([]Skill, len(result.Skills))
	for index, skill := range result.Skills {
		skills[index] = Skill{Name: skill.Name, Status: skill.Status, Path: skill.Path, SHA256: skill.SHA256, Backup: skill.Backup}
	}
	return Output{Status: result.Status, Target: in.Target, Skills: skills, Warnings: result.Warnings, Help: []string{"tadx agent install --target " + in.Target + " --preview"}}, failure
}
