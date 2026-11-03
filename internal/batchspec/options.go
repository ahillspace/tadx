// Package batchspec defines the declarative batch surface shared by registry and CLI.
// It contains no dispatch, application state, or provider dependencies.
package batchspec

// Options describes bounded repetitions of one action, never a workflow.
type Options struct {
	// Selectors are alternative target flags. Only one may vary in shorthand.
	// Native collection properties such as filters and tags are not selectors.
	Selectors []string
	// Identities declares exact target alternatives in priority order. Desired
	// replacement properties must not participate in an existing target's key.
	// Empty defaults to Selectors; positional targets also participate.
	Identities []Identity
	// ResolveTargets requires typed domain bindings to retain authoritative IDs
	// before dispatch, including equivalent ID and exact-name selectors.
	ResolveTargets bool
	// RequestIdentity preserves independent read requests with different filters
	// or projections. Only read-only capabilities use complete request identity.
	RequestIdentity bool
	// NativeSelections names repeated rule inputs that execute separate actions
	// inside one command, without becoming independent target dimensions.
	NativeSelections []string
	// Positional permits args in file rows and repeated single positional targets.
	Positional bool
}

// Identity describes one action-specific conflict domain. If Selector is set,
// this alternative applies only when that flag has a nonempty value. The first
// applicable alternative wins, so an authoritative ID can ignore label fields.
type Identity struct {
	Selector string
	Flags    []string
}
