package datasource

import (
	"github.com/ahillspace/tadx/internal/value"
	"strings"
)

// EnrichFields joins only exact provider identities, never display captions.
// A missing or ambiguous mapping is retained as evidence, not guessed.
func EnrichFields(fields []value.SchemaField, metadata []value.FieldDescription) []value.SchemaField {
	index := make(map[string][]int, len(metadata))
	for i, field := range metadata {
		if field.FullyQualifiedName != "" {
			key := metadataIdentifier(field.FullyQualifiedName)
			index[key] = append(index[key], i)
		}
	}
	out := append([]value.SchemaField(nil), fields...)
	for i := range out {
		out[i].Metadata = nil
		matches := make(map[int]bool)
		for _, key := range []string{out[i].ID, out[i].Name} {
			for _, n := range index[metadataIdentifier(key)] {
				matches[n] = true
			}
		}
		switch len(matches) {
		case 0:
			out[i].MetadataMatch = "unmatched"
		case 1:
			for n := range matches {
				item := metadata[n]
				out[i].Metadata = &item
			}
			out[i].MetadataMatch = "exact"
		default:
			out[i].MetadataMatch = "ambiguous"
		}
	}
	return out
}

// Tableau Metadata wraps simple field identifiers in brackets while VDS returns
// their unquoted spelling. Do not interpret qualified paths or guessed escapes.
func metadataIdentifier(raw string) string {
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		inner := raw[1 : len(raw)-1]
		if inner != "" && !strings.ContainsAny(inner, "[]") {
			return inner
		}
	}
	return raw
}
