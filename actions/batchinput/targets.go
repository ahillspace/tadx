// Package batchinput retains authoritative mutation targets during batch input
// preparation without sharing mutable command bindings between rows.
package batchinput

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver resolves an exact domain selector through the production resource
// adapters. It performs only reads and honors the invocation's selected scope.
type Resolver interface {
	ResolveBatchResource(context.Context, string, string, identity.Selector) (identity.LUID, error)
}

type PreparationAuthorizer interface {
	AuthorizeBatchPreparation(context.Context, string) error
}

// Target identifies the state an action owns, independently of replacement
// values. Environment is shared and resolved before file-row preparation.
type Target struct {
	Environment   string
	Site          string
	WorkspaceID   string
	WorkspaceRoot string
	ResourceKind  string
	ResourceID    string
	Name          string
	PrincipalType string
	PrincipalID   string
	Collection    string
	Capability    string
}

type collectorKey struct{}

type Collector struct {
	operation  string
	targets    []Target
	authorized bool
}

// Collect scopes target preparation to one row. Ordinary command validation has
// no collector and performs no additional resolution through this package.
func Collect(ctx context.Context, operation string) (context.Context, *Collector) {
	c := &Collector{operation: operation}
	return context.WithValue(ctx, collectorKey{}, c), c
}

func Operation(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if c, ok := ctx.Value(collectorKey{}).(*Collector); ok {
		return c.operation
	}
	return ""
}

func (c *Collector) Keys() []string {
	keys := make([]string, 0, len(c.targets))
	for _, target := range c.targets {
		encoded, _ := json.Marshal(target)
		keys = append(keys, string(encoded))
	}
	return keys
}

func Record(ctx context.Context, target Target) {
	if ctx == nil {
		return
	}
	if c, ok := ctx.Value(collectorKey{}).(*Collector); ok {
		c.targets = append(c.targets, target)
	}
}

func Active(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(collectorKey{}).(*Collector)
	return ok
}

// Authorize checks the action's managed capability before a read-only domain
// plan performs setup. Each row reuses that authorization during preparation.
func Authorize(ctx context.Context, resolver Resolver, environment string) error {
	if ctx == nil {
		return nil
	}
	c, ok := ctx.Value(collectorKey{}).(*Collector)
	if !ok || c.authorized {
		return nil
	}
	authorizer, ok := resolver.(PreparationAuthorizer)
	if !ok {
		return errors.New("batch preparation authorization is not configured")
	}
	if err := authorizer.AuthorizeBatchPreparation(ctx, environment); err != nil {
		return err
	}
	c.authorized = true
	return nil
}

// Reference pins a related exact selector without making that related object a
// mutation target, for example the parent of a project being created.
func Reference(ctx context.Context, resolver Resolver, environment, kind string, selector *identity.Selector) error {
	if !Active(ctx) {
		return nil
	}
	return resolve(ctx, resolver, environment, kind, selector)
}

// Resource freezes a resource selector to its authoritative LUID and records the
// mutation identity. Actions retain their normal fresh-state checks afterward.
func Resource(ctx context.Context, resolver Resolver, environment, kind string, selector *identity.Selector) error {
	if !Active(ctx) {
		return nil
	}
	if err := resolve(ctx, resolver, environment, kind, selector); err != nil {
		return err
	}
	Record(ctx, Target{Environment: environment, ResourceKind: kind, ResourceID: string(selector.LUID)})
	return nil
}

// User resolves the principal of a compound mutation without treating that user
// alone as the mutation target. The caller records the full membership or rule.
func User(ctx context.Context, resolver Resolver, environment string, luid, username *string) error {
	if !Active(ctx) {
		return nil
	}
	selector := identity.Selector{LUID: identity.LUID(*luid), Name: *username}
	if err := resolve(ctx, resolver, environment, "user", &selector); err != nil {
		return err
	}
	*luid, *username = string(selector.LUID), ""
	return nil
}

func resolve(ctx context.Context, resolver Resolver, environment, kind string, selector *identity.Selector) error {
	luid := selector.LUID
	if luid == "" {
		if resolver == nil {
			return errors.New("authoritative batch target resolution is not configured")
		}
		var err error
		luid, err = resolver.ResolveBatchResource(ctx, environment, kind, *selector)
		if err != nil {
			return fmt.Errorf("resolve exact batch %s target: %w", kind, err)
		}
	}
	if luid == "" || string(luid) != strings.TrimSpace(string(luid)) {
		return fmt.Errorf("batch %s target has no exact authoritative LUID", kind)
	}
	*selector = identity.Selector{LUID: luid}
	return nil
}
