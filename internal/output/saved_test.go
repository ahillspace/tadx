package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/toon"
)

type directDetailResult struct{ projectableResult }

func (directDetailResult) DetailCommand() []string {
	return []string{"policy", "status", "--full"}
}

func TestDetailExpansionPreservesContextAndSavedResultDefaults(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		saved bool
		args  []string
	}{
		{name: "direct saved", value: directDetailResult{}, saved: true, args: []string{"policy", "status", "--full"}},
		{name: "direct unsaved", value: directDetailResult{}, args: []string{"policy", "status", "--full"}},
		{name: "default saved", value: projectableResult{}, saved: true, args: []string{"last", "--full"}},
		{name: "default unsaved", value: projectableResult{}},
	} {
		for _, jsonOutput := range []bool{false, true} {
			format := "toon"
			if jsonOutput {
				format = "json"
			}
			t.Run(test.name+"/"+format, func(t *testing.T) {
				const configPath = "config path/selected's.yaml"
				options := output.Options{JSON: jsonOutput, ConfigPath: configPath, SavedResult: test.saved}
				var rendered bytes.Buffer
				if err := output.RenderWithOptions(&rendered, test.value, options); err != nil {
					t.Fatal(err)
				}
				var document map[string]any
				if jsonOutput {
					if err := json.Unmarshal(rendered.Bytes(), &document); err != nil {
						t.Fatal(err)
					}
				} else {
					decoded, err := toon.Decode(rendered.Bytes())
					if err != nil {
						t.Fatal(err)
					}
					document = decoded.(map[string]any)
				}
				want := "--full"
				if len(test.args) > 0 {
					args := append([]string{"--config", configPath}, test.args...)
					if jsonOutput {
						args = append(args, "--json")
					}
					want = commandhint.Command(args...)
				}
				if document["details"] != want {
					t.Fatalf("details=%q want=%q", document["details"], want)
				}
				rendered.Reset()
				options.Full = true
				if err := output.RenderWithOptions(&rendered, test.value, options); err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(rendered.Bytes(), []byte("details")) {
					t.Fatalf("full output retained expansion hint: %s", &rendered)
				}
			})
		}
	}
}
