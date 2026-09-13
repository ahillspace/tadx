// Package batchspec defines the declarative batch surface shared by registry and CLI.
// It contains no dispatch, application state, or provider dependencies.
package batchspec

// Options describes bounded repetitions of one action, never a workflow.
type Options struct {
	// Selectors are alternative target flags. Only one may vary in shorthand.
	// Native collection properties such as filters and tags are not selectors.
	Selectors []string
	// NativeSelections names repeated rule inputs that execute separate actions
	// inside one command, without becoming independent target dimensions.
	NativeSelections []string
	// Positional permits args in file rows and repeated single positional targets.
	Positional bool
}
