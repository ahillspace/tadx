package policy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/managedpolicy"
)

// Source supplies the command-scoped administrator policy observation.
type Source interface {
	Status() managedpolicy.Status
	CheckCapability(string) error
}

// Service owns policy installation, candidate validation, samples, and status.
type Service struct{ source Source }

func New(source Source) *Service { return &Service{source: source} }

func (s *Service) InstallManagedPolicy(ctx context.Context, input InstallInput) (InstallOutput, error) {
	input, err := NormalizeInstall(input)
	if err != nil {
		return InstallOutput{}, err
	}
	definitions := capability.All()
	result, err := managedpolicy.Install(ctx, managedpolicy.InstallOptions{Directory: input.OutputDirectory, Template: input.Template}, definitions)
	return installedPolicyOutput(result, err, managedpolicy.InstallationWarnings(result))
}

func installedPolicyOutput(result managedpolicy.InstallResult, installErr error, warnings []string) (InstallOutput, error) {
	output := InstallOutput{
		Path:              filepath.ToSlash(result.Path),
		Template:          result.Template,
		ProtectionChanged: result.ProtectionChanged,
		PolicyWritten:     result.PolicyWritten,
		LocatorPublished:  result.LocatorPublished,
		Active:            result.Active,
		Phase:             result.Phase,
		Warnings:          warnings,
	}
	if installErr != nil {
		return output, policyInstallError(output, installErr)
	}
	return output, nil
}

func policyInstallError(output InstallOutput, cause error) error {
	phase := errs.PhaseSetup
	switch output.Phase {
	case "validation":
		phase = errs.PhaseValidation
	case "write", "locator":
		phase = errs.PhasePersistence
	case "verify":
		phase = errs.PhaseVerification
	}
	completed := make([]string, 0, 4)
	if output.ProtectionChanged {
		completed = append(completed, "directory_protection")
	}
	if output.PolicyWritten {
		completed = append(completed, "policy_write")
	}
	if output.LocatorPublished {
		completed = append(completed, "locator_publish")
	}
	if output.Active {
		completed = append(completed, "policy_active")
	}
	outcome := errs.OutcomeNotAttempted
	correctiveAction := "Review the reported setup or validation problem and see tadx policy install --help for installation guidance. No managed policy change was confirmed."
	if len(completed) > 0 || output.Phase == "write" || output.Phase == "verify" || output.Phase == "locator" || output.Phase == "unknown" {
		outcome = errs.OutcomeUnknown
		correctiveAction = "Inspect the returned path and run tadx policy status. Retain every confirmed change and do not assume the prior policy or locator was restored. See tadx policy install --help for installation guidance."
	}
	return &errs.Error{
		ID:               "policy.install.failed",
		Kind:             errs.KindOperation,
		Operation:        "policy.install",
		Resource:         output.Path,
		Summary:          "Managed policy installation stopped before confirmed completion.",
		Cause:            cause,
		Completed:        completed,
		Phase:            phase,
		Outcome:          outcome,
		Retryable:        errs.Bool(false),
		CorrectiveAction: correctiveAction,
	}
}

func (s *Service) ReadPolicy(context.Context) (StatusOutput, error) {
	state := s.source.Status()
	definitions := capability.All()
	allowed := 0
	for _, definition := range definitions {
		if capability.IsPolicyRecoveryOperation(definition.ID) || s.source.CheckCapability(definition.ID) == nil {
			allowed++
		}
	}
	return StatusOutput{Policy: state, Allowed: allowed, Denied: len(definitions) - allowed, Help: []string{"Policy samples, validate, and status remain available for recovery. Candidate validation does not activate policy or verify protection.", "Remote mutations also require saved consent for the selected Tableau site."}}, nil
}

func (s *Service) ValidateCandidate(ctx context.Context, path string) (ValidationOutput, error) {
	if err := ValidateCandidatePath(path); err != nil {
		return ValidationOutput{}, err
	}
	if err := ctx.Err(); err != nil {
		return ValidationOutput{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ValidationOutput{}, policyToolError("policy.validate", "Candidate policy could not be read.", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return ValidationOutput{}, policyToolError("policy.validate", "Candidate policy could not be read.", err)
	}
	doc, err := managedpolicy.Parse(data, capability.All())
	if err != nil {
		return ValidationOutput{}, &errs.Error{ID: "policy.validate.invalid", Kind: errs.KindUsage, Operation: "policy.validate", Resource: filepath.ToSlash(path), Summary: "Candidate policy is invalid.", Cause: err, Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: "Correct the candidate schema or capability IDs. No policy was activated."}
	}
	return ValidationOutput{Status: "valid", Candidate: filepath.ToSlash(path), AllowedCapabilities: len(doc.AllowedCapabilities), RemoteMutations: doc.RemoteMutations, Help: []string{"Schema and capability IDs are valid. This file is not activated or trusted; an administrator must deploy it with protected ownership and permissions.", "tadx policy status"}}, nil
}

func (s *Service) WriteSamples(ctx context.Context, directory string) (SamplesOutput, error) {
	if err := ValidateSamplesDirectory(directory); err != nil {
		return SamplesOutput{}, err
	}
	path := ""
	if s.source != nil {
		path = s.source.Status().Path
	}
	instructions := []string{"Review one candidate, then run tadx policy validate <file>."}
	if path == "" {
		instructions = append(instructions, "The managed policy location could not be verified. Run tadx policy status and ask an administrator to inspect and repair the locator before deployment; system_path is unknown.")
	} else {
		instructions = append(instructions, "An administrator deploys the selected JSON document at system_path and protects the file and its parent directories against non-administrator changes.")
	}
	instructions = append(instructions, "On Windows, use the native administrator-owned policy location with a protected DACL. On Unix, use root ownership and remove group and other write permissions from the file and its parent directories.", "Run tadx policy status after deployment. These samples do not install or activate policy; local mutation consent remains separate.")
	out := SamplesOutput{Status: "not_created", Files: []string{}, SystemPath: path, Instructions: instructions}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return out, policyToolError("policy.samples", "The output directory could not be created.", err)
	}
	names := []string{"read-only", "read-write-no-admin", "superuser"}
	// Reject existing targets before writing any candidate, then create exclusively
	// to preserve the same protection against races.
	for _, name := range names {
		if _, err := os.Lstat(filepath.Join(directory, name+".json")); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				err = errors.New("candidate already exists")
			}
			return out, policyToolError("policy.samples", "Existing sample files are never overwritten.", err)
		}
	}
	definitions := capability.All()
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return out, policySampleWriteError(out, err)
		}
		doc, err := managedpolicy.Template(name, definitions)
		if err != nil {
			return out, policySampleWriteError(out, err)
		}
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return out, policySampleWriteError(out, err)
		}
		filePath := filepath.Join(directory, name+".json")
		file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return out, policySampleWriteError(out, err)
		}
		_, writeErr := file.Write(append(data, '\n'))
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return out, policySampleWriteError(out, err)
		}
		out.Files = append(out.Files, filepath.ToSlash(filePath))
		out.Status = "partial"
	}
	out.Status = "created"
	return out, nil
}
func policyToolError(operation, summary string, cause error) error {
	return &errs.Error{ID: operation + ".failed", Kind: errs.KindOperation, Operation: operation, Summary: summary, Cause: cause, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: "Review the candidate path and reported error; no active managed policy was changed."}
}
func policySampleWriteError(out SamplesOutput, cause error) error {
	return &errs.Error{ID: "policy.samples.write", Kind: errs.KindOperation, Operation: "policy.samples", Summary: "Policy sample creation stopped.", Cause: cause, Completed: out.Files, Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the returned files and output directory before retrying; existing files are never overwritten."}
}
