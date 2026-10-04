package cli

import (
	"strings"
	"testing"
)

func TestMutationHelpSourceUsesSelectedSiteConsent(t *testing.T) {
	for _, entry := range categoryHelpExamples {
		if entry.Path != "mutation" {
			continue
		}
		if !strings.Contains(entry.Note, "selected server and exact site") || strings.Contains(entry.Note, "TADX_ENABLE_MUTATIONS overrides") {
			t.Fatalf("mutation source must describe selected-site consent: %s", entry.Note)
		}
		if !strings.Contains(entry.Note, "all configured environments") {
			t.Fatalf("mutation status source must describe the inventory: %s", entry.Note)
		}
		for _, example := range entry.Examples {
			if strings.Contains(example, "mutation set") && !strings.Contains(example, "--environment dev") {
				t.Errorf("mutation example lacks selected environment: %s", example)
			}
		}
		return
	}
	t.Fatal("mutation help source is missing")
}
