// Package install installs and activates one administrator-managed policy.
package install

import (
	"context"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	TemplateReadOnly         = "read-only"
	TemplateReadWriteNoAdmin = "read-write-no-admin"
	TemplateSuperuser        = "superuser"
	TemplateAdmin            = "admin"
)

var templates = []string{TemplateReadOnly, TemplateReadWriteNoAdmin, TemplateSuperuser}

type Input struct {
	OutputDirectory string
	Template        string
}

type Output struct {
	Path              string   `json:"path"`
	Template          string   `json:"template"`
	ProtectionChanged bool     `json:"protection_changed"`
	PolicyWritten     bool     `json:"policy_written"`
	LocatorPublished  bool     `json:"locator_published"`
	Active            bool     `json:"active"`
	Phase             string   `json:"phase"`
	Warnings          []string `json:"warnings,omitempty"`
}

type Installer interface {
	InstallManagedPolicy(context.Context, Input) (Output, error)
}

type Action struct {
	installer Installer
}

func New(installer Installer) *Action {
	return &Action{installer: installer}
}

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	input.Template = strings.TrimSpace(input.Template)
	if input.Template == "" || input.Template == TemplateAdmin {
		input.Template = TemplateSuperuser
	}
	if !slices.Contains(templates, input.Template) {
		return Output{}, &errs.Error{
			ID:               "policy.install.usage",
			Kind:             errs.KindUsage,
			Operation:        "policy.install",
			Summary:          "--template must be read-only, read-write-no-admin, or superuser.",
			Phase:            errs.PhaseValidation,
			Outcome:          errs.OutcomeNotAttempted,
			Retryable:        errs.Bool(false),
			CorrectiveAction: "Choose one documented policy template. No policy was installed.",
		}
	}
	if a == nil || a.installer == nil {
		return Output{}, &errs.Error{
			ID:               "policy.install.unconfigured",
			Kind:             errs.KindRuntime,
			Operation:        "policy.install",
			Summary:          "Managed policy installation is not configured.",
			Phase:            errs.PhaseSetup,
			Outcome:          errs.OutcomeNotAttempted,
			Retryable:        errs.Bool(false),
			CorrectiveAction: "Use a supported TADX installation on Windows, Linux, or macOS. No policy was installed.",
		}
	}
	return a.installer.InstallManagedPolicy(ctx, input)
}
