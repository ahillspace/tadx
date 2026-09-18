package value

// SourceContext is the already-resolved origin of a local acquisition.
type SourceContext struct {
	Environment string `json:"environment"`
	Site        string `json:"site"`
}
