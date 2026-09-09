package list

type scopeBoundary struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}
type visibleOutput struct {
	Page         Pagination      `json:"page"`
	Capabilities []Capability    `json:"capabilities"`
	OutOfScope   []scopeBoundary `json:"out_of_scope,omitempty"`
	Help         []string        `json:"help"`
}

func (o Output) CompactOutput() any {
	result := visibleOutput{Page: o.Page, Capabilities: []Capability{}, Help: o.Help}
	for _, item := range o.Capabilities {
		if item.Disposition == "delegated" {
			result.OutOfScope = append(result.OutOfScope, scopeBoundary{item.ID, "Out of scope"})
		} else {
			result.Capabilities = append(result.Capabilities, item)
		}
	}
	return result
}
func (o Output) FullOutput() any { return o.CompactOutput() }
