package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	policyinstall "github.com/ahillspace/tadx/actions/policy/install"
	policysamples "github.com/ahillspace/tadx/actions/policy/samples"
	policystatus "github.com/ahillspace/tadx/actions/policy/status"
	policyvalidate "github.com/ahillspace/tadx/actions/policy/validate"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	policycli "github.com/ahillspace/tadx/internal/cli/policy"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/lastcommand"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/value"
	"github.com/spf13/cobra"
)

type managedPolicySource interface {
	Status() managedpolicy.Status
	CheckCapability(string) error
	CheckRemoteMutation() error
}

type managedCapabilityChecks struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

func (c *managedCapabilityChecks) record(id string) {
	if id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = map[string]struct{}{}
	}
	c.ids[id] = struct{}{}
}
func (c *managedCapabilityChecks) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Sorted(maps.Keys(c.ids))
}

type managedLastReader struct {
	store   lastcommand.Store
	runtime *runtimeDependencies
}

func (r managedLastReader) Read(ctx context.Context) (value.SavedExecution, error) {
	record, err := r.store.Read(ctx)
	if err != nil {
		return record, err
	}
	if err := r.runtime.checkManagedCapability(record.Operation); err != nil {
		return value.SavedExecution{}, err
	}
	for _, id := range record.RequiredCapabilities {
		if err := r.runtime.checkManagedCapability(id); err != nil {
			return value.SavedExecution{}, err
		}
	}
	if record.Operation == "search.run" {
		for _, id := range legacySearchPrerequisites(record.Result) {
			if err := r.runtime.checkManagedCapability(id); err != nil {
				return value.SavedExecution{}, err
			}
		}
	}
	return record, nil
}

// Old search snapshots have no prerequisite ledger. Inspect only the known
// search row shape, including the partial-result envelope, never owner fields.
func legacySearchPrerequisites(data json.RawMessage) []string {
	type row struct {
		Type string `json:"type"`
	}
	type searchSnapshot struct {
		Items []row `json:"items"`
	}
	var snapshot struct {
		Items  []row          `json:"items"`
		Output searchSnapshot `json:"output"`
	}
	if json.Unmarshal(data, &snapshot) != nil {
		return nil
	}
	ids := map[string]struct{}{}
	for _, item := range append(snapshot.Items, snapshot.Output.Items...) {
		switch item.Type {
		case "user":
			ids["admin.user.list"] = struct{}{}
		case "group":
			ids["admin.group.list"] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(ids))
}

func (s registrySource) applyManagedDiscovery(item *capability.Discovery) {
	if s.runtime == nil || s.runtime.managedPolicy == nil || policyRecoveryOperation(item.ID) {
		return
	}
	err := s.runtime.managedPolicy.CheckCapability(item.ID)
	if err == nil && item.RemoteMutation {
		err = s.runtime.managedPolicy.CheckRemoteMutation()
	}
	if err != nil {
		item.PolicyDenied = true
		item.PolicyReason = err.Error()
		item.ExecutionEnabled = false
	}
}

func policyRecoveryOperation(id string) bool {
	return id == "policy.install" || id == "policy.samples" || id == "policy.validate" || id == "policy.status"
}

func (r *runtimeDependencies) checkManagedCapability(id string) error {
	if r == nil {
		return nil
	}
	if policyRecoveryOperation(id) || r.managedPolicy == nil {
		r.managedChecks.record(id)
		return nil
	}
	if err := r.managedPolicy.CheckCapability(id); err != nil {
		return managedPolicyError(id, err)
	}
	r.managedChecks.record(id)
	return nil
}

func managedPolicyError(id string, cause error) error {
	return &errs.Error{ID: "policy.denied", Kind: errs.KindOperation, Operation: id, Resource: id, Summary: "The administrator-managed policy blocks this operation.", Cause: cause, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: "Run tadx policy status for the fixed policy path and diagnostics. An administrator must repair or update that policy; local settings cannot override it."}
}

// Install after publication dispatch wrapping so denied work cannot launch a worker.
// Workers call Run again and load the protected policy independently.
func bindManagedPolicy(root *cobra.Command, r *runtimeDependencies) {
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if command.Annotations["tadx.grouping"] != "true" && (command.RunE != nil || command.Run != nil) {
			originalE, original := command.RunE, command.Run
			command.Run = nil
			command.RunE = func(c *cobra.Command, args []string) error {
				id := c.Annotations[cli.CapabilityAnnotation]
				if c == root {
					if version, _ := c.Flags().GetBool("version"); version {
						id = "version.get"
					}
				}
				if err := r.checkManagedCapability(id); err != nil {
					return err
				}
				if definition, ok := capability.Lookup(id); ok && definition.RemoteMutation && r.managedPolicy != nil {
					preview, _ := c.Flags().GetBool("preview")
					if !preview {
						if err := r.managedPolicy.CheckRemoteMutation(); err != nil {
							return managedPolicyError(id, err)
						}
					}
				}
				if originalE != nil {
					return originalE(c, args)
				}
				original(c, args)
				return nil
			}
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}

func (r *runtimeDependencies) policyDependencies() *policycli.Dependencies {
	return &policycli.Dependencies{Installer: policyinstall.New(r), Sampler: policysamples.New(r), Validator: policyvalidate.New(r), Statuser: policystatus.New(r)}
}

func (r *runtimeDependencies) InstallManagedPolicy(ctx context.Context, input policyinstall.Input) (policyinstall.Output, error) {
	result, err := managedpolicy.Install(ctx, managedpolicy.InstallOptions{Directory: input.OutputDirectory, Template: input.Template}, capability.All())
	output := policyinstall.Output{
		Path:              filepath.ToSlash(result.Path),
		Template:          result.Template,
		ProtectionChanged: result.ProtectionChanged,
		PolicyWritten:     result.PolicyWritten,
		LocatorPublished:  result.LocatorPublished,
		Active:            result.Active,
		Phase:             result.Phase,
	}
	if err != nil {
		return output, policyInstallError(output, err)
	}
	state := managedpolicy.Load(capability.All()).Status()
	if strings.EqualFold(filepath.Clean(state.Path), filepath.Clean(result.Path)) {
		output.Warnings = state.Warnings
	}
	return output, nil
}

func policyInstallError(output policyinstall.Output, cause error) error {
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
	correctiveAction := "Correct the reported setup or validation problem, then rerun the install command. No managed policy change was confirmed."
	if len(completed) > 0 || output.Phase == "write" || output.Phase == "verify" || output.Phase == "locator" || output.Phase == "unknown" {
		outcome = errs.OutcomeUnknown
		correctiveAction = "Inspect the returned path and run tadx policy status. Retain every confirmed change and do not assume the prior policy or locator was restored."
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

func (r *runtimeDependencies) ReadPolicy(context.Context) (policystatus.Output, error) {
	state := r.managedPolicy.Status()
	allowed := 0
	for _, definition := range capability.All() {
		if policyRecoveryOperation(definition.ID) || r.managedPolicy.CheckCapability(definition.ID) == nil {
			allowed++
		}
	}
	return policystatus.Output{Policy: state, Allowed: allowed, Denied: len(capability.All()) - allowed, Help: []string{"Policy samples, validate, and status remain available for recovery. Candidate validation does not activate policy or verify protection.", "Remote mutations also require saved consent for the selected Tableau site."}}, nil
}

func (r *runtimeDependencies) ValidateCandidate(ctx context.Context, path string) (policyvalidate.Output, error) {
	if err := ctx.Err(); err != nil {
		return policyvalidate.Output{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return policyvalidate.Output{}, policyToolError("policy.validate", "Candidate policy could not be read.", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return policyvalidate.Output{}, policyToolError("policy.validate", "Candidate policy could not be read.", err)
	}
	doc, err := managedpolicy.Parse(data, capability.All())
	if err != nil {
		return policyvalidate.Output{}, &errs.Error{ID: "policy.validate.invalid", Kind: errs.KindUsage, Operation: "policy.validate", Resource: filepath.ToSlash(path), Summary: "Candidate policy is invalid.", Cause: err, Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: "Correct the candidate schema or capability IDs. No policy was activated."}
	}
	return policyvalidate.Output{Status: "valid", Candidate: filepath.ToSlash(path), AllowedCapabilities: len(doc.AllowedCapabilities), RemoteMutations: doc.RemoteMutations, Help: []string{"Schema and capability IDs are valid. This file is not activated or trusted; an administrator must deploy it with protected ownership and permissions.", "tadx policy status"}}, nil
}

func (r *runtimeDependencies) WriteSamples(ctx context.Context, directory string) (policysamples.Output, error) {
	path, err := managedpolicy.SystemPath()
	if err != nil {
		return policysamples.Output{}, policyToolError("policy.samples", "The fixed policy location is unavailable.", err)
	}
	out := policysamples.Output{Status: "not_created", Files: []string{}, SystemPath: path, Instructions: []string{"Review one candidate, then run tadx policy validate <file>.", "An administrator deploys the selected JSON document at system_path and protects the file and its parent directories against non-administrator changes.", "On Windows, use the native administrator-owned policy location with a protected DACL. On Unix, use root ownership and remove group and other write permissions from the file and its parent directories.", "Run tadx policy status after deployment. These samples do not install or activate policy; local mutation consent remains separate."}}
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
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return out, policySampleWriteError(out, err)
		}
		doc, err := managedpolicy.Template(name, capability.All())
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
func policySampleWriteError(out policysamples.Output, cause error) error {
	return &errs.Error{ID: "policy.samples.write", Kind: errs.KindOperation, Operation: "policy.samples", Summary: "Policy sample creation stopped.", Cause: cause, Completed: out.Files, Phase: errs.PhasePersistence, Outcome: errs.OutcomeUnknown, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the returned files and output directory before retrying; existing files are never overwritten."}
}
