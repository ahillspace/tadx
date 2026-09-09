package pull

import (
	"fmt"
	"strings"
)

// NormalizeInput checks local lineage selectors and bounds before setup.
func NormalizeInput(input Input) (Input, error) {
	input.Kind = strings.TrimSpace(input.Kind)
	input.Direction = strings.TrimSpace(input.Direction)
	if input.Kind != "workbook" && input.Kind != "published_datasource" && input.Kind != "flow" {
		return Input{}, usageCause("kind", "unsupported lineage root kind", fmt.Errorf("unsupported lineage root kind %q", input.Kind))
	}
	if input.Selector.LUID == "" && strings.TrimSpace(input.Selector.Name) == "" {
		return Input{}, usage("selector", "lineage pull requires a REST LUID or exact name selector")
	}
	if input.Direction == "" {
		input.Direction = "both"
	}
	if input.Direction != "upstream" && input.Direction != "downstream" && input.Direction != "both" {
		return Input{}, usageCause("direction", "unsupported lineage direction", fmt.Errorf("unsupported lineage direction %q", input.Direction))
	}
	if input.Depth == 0 {
		input.Depth = 1
	}
	if input.Depth < 1 || input.Depth > 3 {
		return Input{}, usage("depth", "lineage depth must be between 1 and 3")
	}
	return input, nil
}

func ValidateInput(input Input) error { _, err := NormalizeInput(input); return err }
