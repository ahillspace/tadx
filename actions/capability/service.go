// Package capability owns registry discovery and readiness projection.
package capability

import (
	"context"
	"strings"

	registry "github.com/ahillspace/tadx/internal/capability"
)

// ManagedPolicy supplies the current administrator policy observations.
type ManagedPolicy interface {
	CheckCapability(string) error
	CheckRemoteMutation() error
}

// Ports supplies readiness observations without command syntax or native operations.
type Ports struct {
	ManagedPolicy         ManagedPolicy
	ResolveMutationPolicy func(string) (bool, string, error)
}

// Service owns named capability discovery operations.
type Service struct{ ports Ports }

func New(ports Ports) *Service { return &Service{ports: ports} }

func (s *Service) ListCapabilities(_ context.Context, input ListInput) (ListOutput, error) {
	enabled, policyErr := s.mutationReadiness(input.Environment)
	items := registry.AllDiscoveries()
	for index := range items {
		s.applyManagedReadiness(&items[index])
	}
	output, err := listFromItems(input, items, enabled)
	if policyErr != nil {
		if err != nil {
			return ListOutput{}, policyErr
		}
		output.MutationPolicy = "unavailable"
		return output, policyErr
	}
	return output, err
}

func (s *Service) GetCapability(_ context.Context, input GetInput) (GetOutput, error) {
	enabled, err := s.mutationReadiness(input.Environment)
	if err != nil {
		return GetOutput{}, err
	}
	item, ok := registry.LookupDiscovery(strings.TrimSpace(input.ID))
	if ok {
		s.applyManagedReadiness(&item)
	}
	return getFromItem(input, item, ok, enabled)
}

func (s *Service) mutationReadiness(environment string) (bool, error) {
	if s == nil || s.ports.ResolveMutationPolicy == nil {
		return false, nil
	}
	enabled, _, err := s.ports.ResolveMutationPolicy(environment)
	if err != nil {
		return false, err
	}
	return enabled, err
}

func (s *Service) applyManagedReadiness(item *registry.Discovery) {
	if s == nil || s.ports.ManagedPolicy == nil || registry.IsPolicyRecoveryOperation(item.ID) {
		return
	}
	err := s.ports.ManagedPolicy.CheckCapability(item.ID)
	if err == nil && item.RemoteMutation {
		err = s.ports.ManagedPolicy.CheckRemoteMutation()
	}
	if err != nil {
		item.PolicyDenied = true
		item.PolicyReason = err.Error()
		item.ExecutionEnabled = false
	}
}
