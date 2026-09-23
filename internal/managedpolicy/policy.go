// Package managedpolicy enforces an administrator-owned, machine-wide ceiling.
// It never grants implementation readiness or enables the local mutation setting.
package managedpolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/value"
)

const maxPolicyBytes = 1 << 20

const (
	StateUnmanaged = "unmanaged"
	StateActive    = "active"
	StateError     = "error"
	StateBlocked   = StateError
)

var (
	ErrBlocked              = errors.New("managed policy blocks operations")
	ErrCapabilityDenied     = errors.New("capability denied by managed policy")
	ErrRemoteMutationDenied = errors.New("remote mutations denied by managed policy")
)

// Document is a candidate policy. Parsing it does not activate or trust it.
type Document struct {
	Version             int      `json:"version"`
	AllowedCapabilities []string `json:"allowed_capabilities"`
	RemoteMutations     bool     `json:"remote_mutations"`
}

type ProtectionCheck = value.ManagedPolicyProtectionCheck

type Status = value.ManagedPolicyStatus

// Policy is an immutable snapshot loaded from the fixed system location.
// Its zero value blocks operations. Load again at each command/worker boundary.
type Policy struct {
	status  Status
	allowed map[string]struct{}
}

// Parse validates a candidate independently of its filesystem protection.
func Parse(data []byte, definitions []capability.Definition) (Document, error) {
	var doc Document
	if len(data) > maxPolicyBytes || !utf8.Valid(data) {
		return doc, errors.New("policy must be valid UTF-8 and at most 1 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return doc, errors.New("policy must be a JSON object")
	}
	fields := make(map[string]json.RawMessage, 3)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return doc, errors.New("invalid policy field")
		}
		key, ok := token.(string)
		if !ok {
			return doc, errors.New("invalid policy field")
		}
		if _, exists := fields[key]; exists {
			return doc, fmt.Errorf("duplicate policy field %q", key)
		}
		if key != "version" && key != "allowed_capabilities" && key != "remote_mutations" {
			return doc, errors.New("unknown policy field")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return doc, errors.New("invalid policy field value")
		}
		fields[key] = raw
	}
	if token, err = dec.Token(); err != nil || token != json.Delim('}') {
		return doc, errors.New("invalid policy object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return doc, errors.New("policy contains trailing JSON or data")
	}
	for _, key := range []string{"version", "allowed_capabilities", "remote_mutations"} {
		value, exists := fields[key]
		if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return doc, fmt.Errorf("policy requires non-null %s", key)
		}
	}
	if err := json.Unmarshal(fields["version"], &doc.Version); err != nil || doc.Version != 1 {
		return Document{}, errors.New("unsupported policy version; expected integer 1")
	}
	if err := json.Unmarshal(fields["allowed_capabilities"], &doc.AllowedCapabilities); err != nil {
		return Document{}, errors.New("allowed_capabilities must be an array of capability IDs")
	}
	if err := json.Unmarshal(fields["remote_mutations"], &doc.RemoteMutations); err != nil {
		return Document{}, errors.New("remote_mutations must be a boolean")
	}
	known := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		known[definition.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(doc.AllowedCapabilities))
	for _, id := range doc.AllowedCapabilities {
		if _, exists := known[id]; !exists || id == "" {
			return Document{}, errors.New("allowed_capabilities contains an unknown capability ID")
		}
		if _, exists := seen[id]; exists {
			return Document{}, fmt.Errorf("duplicate capability ID %q", id)
		}
		seen[id] = struct{}{}
	}
	slices.Sort(doc.AllowedCapabilities)
	return doc, nil
}

// Template snapshots current IDs, including preview-capable remote mutations.
// Future IDs remain denied until an administrator updates the explicit list.
func Template(name string, definitions []capability.Definition) (Document, error) {
	if name == "admin" {
		name = "superuser"
	}
	if name != "read-only" && name != "read-write-no-admin" && name != "superuser" {
		return Document{}, errors.New("unknown policy template")
	}
	doc := Document{Version: 1, AllowedCapabilities: []string{}, RemoteMutations: name != "read-only"}
	for _, definition := range definitions {
		// Templates restrict mutations, not administrative information retrieval.
		if name == "read-write-no-admin" && definition.Administrative && definition.RemoteMutation {
			continue
		}
		doc.AllowedCapabilities = append(doc.AllowedCapabilities, definition.ID)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return Document{}, err
	}
	return Parse(data, definitions)
}

// Load never consults environment variables or user configuration for discovery.
func Load(definitions []capability.Definition) *Policy {
	path, required, err := systemPolicyLocation()
	return loadResolved(path, required, err, definitions)
}

func loadResolved(path string, required bool, err error, definitions []capability.Definition) *Policy {
	if err != nil {
		return &Policy{status: Status{State: StateBlocked, Reason: "system policy location is unavailable: " + err.Error()}}
	}
	p := loadPath(path, definitions)
	if required && p.status.State == StateUnmanaged {
		p.status.State = StateBlocked
		p.status.RemoteMutations = false
		p.status.Reason = "the installed policy locator points to a missing policy; run policy install to repair it"
	}
	return p
}

func loadPath(path string, definitions []capability.Definition) *Policy {
	p := &Policy{status: Status{State: StateBlocked, Path: path, Checks: []ProtectionCheck{}, AllowedCapabilities: []string{}}}
	data, checks, err := secureRead(path)
	p.status.Checks = checks
	if errors.Is(err, os.ErrNotExist) {
		p.status.State = StateUnmanaged
		p.status.RemoteMutations = true
		return p
	}
	if err != nil {
		p.status.Warnings = ancestorWarnings(checks)
		p.status.Reason = err.Error()
		return p
	}
	p.status.Protected = true
	p.status.PathProtected = allProtectionChecksPassed(checks)
	p.status.Warnings = ancestorWarnings(checks)
	doc, err := Parse(data, definitions)
	if err != nil {
		p.status.Reason = err.Error()
		return p
	}
	active := activePolicy(path, doc)
	active.status.Checks = checks
	active.status.PathProtected = p.status.PathProtected
	active.status.Warnings = p.status.Warnings
	return active
}

func allProtectionChecksPassed(checks []ProtectionCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func ancestorWarnings(checks []ProtectionCheck) []string {
	for _, check := range checks {
		if check.Kind == "ancestor-owner-acl-and-links" && !check.Passed {
			return []string{fmt.Sprintf("Managed policy path warning: ancestor %q is not verified as protected (%s). The policy path may be replaced and a different policy substituted. Ask an administrator to move the policy to a protected path or review this ancestor's ACL; run tadx policy status --full for all checks.", check.Path, check.Reason)}
		}
	}
	return nil
}

func activePolicy(path string, doc Document) *Policy {
	p := &Policy{status: Status{State: StateActive, Path: path, CandidateValid: true, Protected: true, PathProtected: true, AllowedCapabilities: slices.Clone(doc.AllowedCapabilities), RemoteMutations: doc.RemoteMutations}, allowed: make(map[string]struct{}, len(doc.AllowedCapabilities))}
	for _, id := range doc.AllowedCapabilities {
		p.allowed[id] = struct{}{}
	}
	return p
}

func (p *Policy) Status() Status {
	if p == nil {
		return Status{State: StateBlocked, Reason: "managed policy is not initialized"}
	}
	s := p.status
	if s.State == "" {
		s.State = StateBlocked
		s.Reason = "managed policy is not initialized"
	}
	s.Checks = slices.Clone(s.Checks)
	s.Warnings = slices.Clone(s.Warnings)
	s.AllowedCapabilities = slices.Clone(s.AllowedCapabilities)
	return s
}

func (p *Policy) CheckCapability(id string) error {
	if p == nil || (p.status.State != StateActive && p.status.State != StateUnmanaged) {
		return ErrBlocked
	}
	if p.status.State == StateUnmanaged {
		return nil
	}
	if _, allowed := p.allowed[id]; !allowed {
		return fmt.Errorf("%w: %s", ErrCapabilityDenied, id)
	}
	return nil
}

func (p *Policy) CheckRemoteMutation() error {
	if p == nil || (p.status.State != StateActive && p.status.State != StateUnmanaged) {
		return ErrBlocked
	}
	if !p.status.RemoteMutations {
		return ErrRemoteMutationDenied
	}
	return nil
}

func readBounded(file *os.File) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxPolicyBytes+1))
	if err != nil {
		return nil, errors.New("cannot read managed policy")
	}
	if len(data) > maxPolicyBytes {
		return nil, errors.New("managed policy exceeds 1 MiB")
	}
	return data, nil
}
