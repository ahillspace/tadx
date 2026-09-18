package list

import "github.com/ahillspace/tadx/internal/capability"

type scopeBoundary struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}
type visibleOutput struct {
	Page           Pagination           `json:"page"`
	Capabilities   []capability.Summary `json:"capabilities"`
	Counts         Counts               `json:"counts"`
	MutationPolicy string               `json:"mutation_policy,omitempty"`
	NextCommand    string               `json:"next_command,omitempty"`
	OutOfScope     []scopeBoundary      `json:"out_of_scope,omitempty"`
	Help           []string             `json:"help"`
}

func (o Output) CompactOutput() any {
	result := visibleOutput{
		Page: o.Page, Capabilities: []capability.Summary{}, Counts: o.Counts,
		MutationPolicy: o.MutationPolicy, NextCommand: o.NextCommand, Help: o.Help,
	}
	for _, item := range o.Capabilities {
		if item.Disposition == "delegated" {
			result.OutOfScope = append(result.OutOfScope, scopeBoundary{item.ID, "Out of scope"})
		} else {
			result.Capabilities = append(result.Capabilities, item.Summary())
		}
	}
	return result
}

// FullOutput retains the same bounded page and expands each row to the
// contract representation shared by capability get.
func (o Output) FullOutput() any {
	return struct {
		Page           Pagination   `json:"page"`
		Capabilities   []Capability `json:"capabilities"`
		Counts         Counts       `json:"counts"`
		MutationPolicy string       `json:"mutation_policy,omitempty"`
		NextCommand    string       `json:"next_command,omitempty"`
		Help           []string     `json:"help"`
	}{
		Page: o.Page, Capabilities: o.Capabilities, Counts: o.Counts,
		MutationPolicy: o.MutationPolicy, NextCommand: o.NextCommand, Help: o.Help,
	}
}
