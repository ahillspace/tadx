package cli

import (
	"strings"
	"testing"
)

func TestMutationHelpSourceUsesSelectedSiteConsent(t *testing.T) {
	for _, entry := range categoryHelpExamples {
		if entry.path != "mutation" {
			continue
		}
		if !strings.Contains(entry.note, "selected server and exact site") || strings.Contains(entry.note, "TADX_ENABLE_MUTATIONS overrides") {
			t.Fatalf("mutation source must describe selected-site consent: %s", entry.note)
		}
		for _, example := range entry.examples {
			if !strings.Contains(example, "--environment dev") {
				t.Errorf("mutation example lacks selected environment: %s", example)
			}
		}
		return
	}
	t.Fatal("mutation help source is missing")
}
