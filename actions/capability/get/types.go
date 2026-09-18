package get

import "github.com/ahillspace/tadx/internal/capability"

// Input selects one capability by its exact registry ID.
type Input struct {
	ID               string `json:"id"`
	MutationsEnabled bool   `json:"-"`
}

// Capability is the detailed discovery view of one registry entry.
//
// The neutral representation is shared with capability.list full output so
// that the two commands cannot drift in their contract fields.
type Capability = capability.Discovery

// Output is the stable capability detail result.
type Output struct {
	Capability Capability `json:"capability"`
	Help       []string   `json:"help"`
}
