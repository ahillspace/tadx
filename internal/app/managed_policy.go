package app

import (
	"maps"
	"slices"
	"sync"

	policyops "github.com/ahillspace/tadx/actions/policy"
	"github.com/ahillspace/tadx/internal/capability"
	policycli "github.com/ahillspace/tadx/internal/cli/policy"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/managedpolicy"
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

func (r *runtimeDependencies) checkManagedCapability(id string) error {
	if r == nil {
		return nil
	}
	if capability.IsPolicyRecoveryOperation(id) || r.managedPolicy == nil {
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

func (r *runtimeDependencies) checkManagedRemoteMutation(id string) error {
	definition, ok := capability.Lookup(id)
	if !ok || !definition.RemoteMutation || r.managedPolicy == nil {
		return nil
	}
	if err := r.managedPolicy.CheckRemoteMutation(); err != nil {
		return managedPolicyError(id, err)
	}
	return nil
}

func (r *runtimeDependencies) policyDependencies() *policycli.Dependencies {
	service := policyops.New(r.managedPolicy)
	return &policycli.Dependencies{Installer: service, Sampler: service, Validator: service, Statuser: service}
}
