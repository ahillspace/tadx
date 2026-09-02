package output

const (
	maxBoundedWarnings     = 20
	maxBoundedWarningRunes = 512
)

// BoundWarnings caps a warning slice to a bounded count and truncates over-long entries.
func BoundWarnings(values []string) []string {
	if len(values) > maxBoundedWarnings {
		values = values[:maxBoundedWarnings]
	}
	bounded := make([]string, len(values))
	for index, value := range values {
		runes := []rune(value)
		if len(runes) > maxBoundedWarningRunes {
			value = string(runes[:maxBoundedWarningRunes-3]) + "..."
		}
		bounded[index] = value
	}
	return bounded
}
