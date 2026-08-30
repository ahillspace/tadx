// Package toon implements the TOON 4.1 JSON data-model codec.
//
// Numbers decode to json.Number. Values outside Go's finite float64 domain
// remain lossless decimal json.Number values. Encoding normalizes decimal
// spelling while preserving mathematical value.
package toon

const (
	// SpecVersion is the pinned upstream TOON specification version.
	SpecVersion = "4.1"
	// FixtureRevision identifies the upstream fixture release audited by tests.
	FixtureRevision = "v4.1.1"
)

// EncodeOptions configures TOON encoding.
type EncodeOptions struct {
	IndentSize int
	Delimiter  rune
}

// DecodeOptions configures TOON decoding. Decode uses strict mode by default.
type DecodeOptions struct {
	IndentSize int
	Strict     bool
}
