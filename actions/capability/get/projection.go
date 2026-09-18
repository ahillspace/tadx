package get

type visibleOutput Output

func (o Output) CompactOutput() any {
	if o.Capability.Disposition == "delegated" {
		o.Capability.Disposition = "Out of scope"
		o.Capability.ImplementationState = "out_of_scope"
		o.Capability.Command = ""
		o.Capability.Surface = "Out of scope"
		o.Capability.SafetyGuard = "Out of scope. TADX does not execute or hand off this operation."
	}
	return visibleOutput(o)
}

// FullOutput retains the complete bounded contract, including delegated
// capabilities, without applying the compact out-of-scope projection.
func (o Output) FullOutput() any { return visibleOutput(o) }
