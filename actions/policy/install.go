// Package policy owns managed policy operation contracts and projections.
package policy

import (
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

type InstallInput struct {
	OutputDirectory string
	Template        string
}

type InstallOutput struct {
	Path              string   `json:"path"`
	Template          string   `json:"template"`
	ProtectionChanged bool     `json:"protection_changed"`
	PolicyWritten     bool     `json:"policy_written"`
	LocatorPublished  bool     `json:"locator_published"`
	Active            bool     `json:"active"`
	Phase             string   `json:"phase"`
	Warnings          []string `json:"warnings,omitempty"`
}

// NormalizeInstall validates and resolves the requested template before native setup.
func NormalizeInstall(input InstallInput) (InstallInput, error) {
	input.Template = strings.TrimSpace(input.Template)
	if input.Template == "" || input.Template == TemplateAdmin {
		input.Template = TemplateSuperuser
	}
	if !slices.Contains(templates, input.Template) {
		return InstallInput{}, &errs.Error{
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
	return input, nil
}
