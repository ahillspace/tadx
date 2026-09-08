package capability

import "encoding/json"

// GenerateJSON exports deterministic capability facts for external visualizations.
// It contains data only; it does not generate or change a visualization's UI.
func GenerateJSON(definitions []Definition) ([]byte, error) {
	if err := Validate(definitions); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(sorted(append([]Definition(nil), definitions...)), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
