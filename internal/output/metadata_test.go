package output_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func TestMetadataSurvivesProjectionAndRedaction(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, asJSON := range []bool{false, true} {
			var out bytes.Buffer
			err := output.RenderWithOptions(&out, projectableResult{Status: "ready", Secret: "private-value"}, output.Options{JSON: asJSON, Full: full, Secrets: []string{"private-value"}, Metadata: map[string]any{"warnings": []string{"broken profile: private-value"}}})
			if err != nil || !strings.Contains(out.String(), "metadata") || !strings.Contains(out.String(), "warnings") || strings.Contains(out.String(), "private-value") {
				t.Fatalf("metadata projection or redaction failed: out=%s err=%v", out.String(), err)
			}
			if asJSON && !json.Valid(out.Bytes()) {
				t.Fatal("metadata invalidated JSON output")
			}
		}
	}
}

func TestMetadataMergePreservesResultAndExistingDiagnostics(t *testing.T) {
	value := map[string]any{"status": "ready", "metadata": map[string]any{"warnings": []string{"existing"}, "coverage": "partial"}}
	metadata := map[string]any{"warnings": []string{"broken"}}
	var out bytes.Buffer
	if err := output.RenderWithOptions(&out, value, output.Options{JSON: true, Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	var document struct {
		Status   string `json:"status"`
		Metadata struct {
			Warnings []string `json:"warnings"`
			Coverage string   `json:"coverage"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(out.Bytes(), &document); err != nil || document.Status != "ready" || document.Metadata.Coverage != "partial" || strings.Join(document.Metadata.Warnings, ",") != "existing,broken" {
		t.Fatalf("result diagnostics were overwritten: %s err=%v", out.Bytes(), err)
	}
	if len(value["metadata"].(map[string]any)["warnings"].([]string)) != 1 || len(metadata["warnings"].([]string)) != 1 {
		t.Fatal("rendering mutated the caller's metadata")
	}
}

func TestErrorMetadataRemainsOutsideErrorPayload(t *testing.T) {
	var out bytes.Buffer
	if err := output.RenderError(&out, errs.New(errs.KindOperation, "selected operation failed"), output.Options{JSON: true, Metadata: map[string]any{"warnings": []string{"unrelated invalid profile"}}}); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document["metadata"] == nil || document["error"].(map[string]any)["metadata"] != nil {
		t.Fatalf("warning metadata entered unrelated error payload: %s", out.Bytes())
	}
}

func TestSnapshotMetadataIsRedactedAndBounded(t *testing.T) {
	options := output.Options{Metadata: map[string]any{"warnings": []string{"broken profile"}, "token": "credential-value", "diagnostic": "secret-value"}, Secrets: []string{"secret-value"}}
	data, err := output.SnapshotWithOptions(projectableResult{Status: "ready", Secret: "secret-value"}, 4096, options)
	if err != nil || !json.Valid(data) || !strings.Contains(string(data), "broken profile") || strings.Contains(string(data), "credential-value") || strings.Contains(string(data), "secret-value") {
		t.Fatalf("saved metadata lost redaction: data=%s err=%v", data, err)
	}
	options.Metadata = map[string]any{"warnings": []string{strings.Repeat("x", 512)}}
	if _, err := output.SnapshotWithOptions(map[string]string{"status": "ready"}, 100, options); err == nil {
		t.Fatal("metadata bypassed the saved result size bound")
	}
}

func TestSavedDetailHintPreservesConfigurationInStructuredRecovery(t *testing.T) {
	const configPath = "selected config/settings.yaml"
	value := errs.Envelope{Error: errs.Payload{Kind: errs.KindOperation, Summary: "Invalid environment.", CorrectiveCommands: [][]string{{"env", "update", "broken"}, {"env", "remove", "broken"}}}}
	var out bytes.Buffer
	if err := output.RenderWithOptions(&out, value, output.Options{JSON: true, ConfigPath: configPath, SavedResult: true}); err != nil {
		t.Fatal(err)
	}
	var document struct {
		Error struct {
			CorrectiveAction string `json:"corrective_action"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &document); err != nil || strings.Count(document.Error.CorrectiveAction, "--config") != 2 || strings.Count(document.Error.CorrectiveAction, configPath) != 2 {
		t.Fatalf("detail hint removed repair configuration: out=%s err=%v", out.Bytes(), err)
	}
}

type metadataRecoveryCarrier struct{}

func (metadataRecoveryCarrier) Error() string { return "The environment entry is invalid." }
func (metadataRecoveryCarrier) CorrectiveCommands() [][]string {
	return [][]string{{"env", "update", "--", "-broken alias'fixture"}, {"env", "remove", "--", "-broken alias'fixture"}}
}
func (metadataRecoveryCarrier) CorrectiveExplanation() string {
	return "Review secret-value before retrying."
}

func TestStructuredRecoveryPreservesArgumentsAndRedactionAcrossOutputModes(t *testing.T) {
	for _, configPath := range []string{"", "selected config/settings'fixture.yaml"} {
		for _, full := range []bool{false, true} {
			for _, snapshot := range []bool{false, true} {
				t.Run(fmt.Sprintf("config_%t_full_%t_snapshot_%t", configPath != "", full, snapshot), func(t *testing.T) {
					failure := &errs.Error{Kind: errs.KindOperation, Summary: "Environment selection failed.", Cause: metadataRecoveryCarrier{}}
					options := output.Options{JSON: true, Full: full, ConfigPath: configPath, SavedResult: true, Secrets: []string{"secret-value"}}
					var data []byte
					if snapshot {
						var err error
						data, err = output.SnapshotWithOptions(failure, 4096, options)
						if err != nil {
							t.Fatal(err)
						}
					} else {
						var out bytes.Buffer
						if err := output.RenderWithOptions(&out, errs.Structure(failure), options); err != nil {
							t.Fatal(err)
						}
						data = out.Bytes()
					}
					var document struct {
						Error struct {
							CorrectiveAction string `json:"corrective_action"`
						} `json:"error"`
					}
					if err := json.Unmarshal(data, &document); err != nil {
						t.Fatal(err)
					}
					for _, args := range (metadataRecoveryCarrier{}).CorrectiveCommands() {
						if configPath != "" {
							args = append([]string{"--config", configPath}, args...)
						}
						if expected := commandhint.Command(args...); !strings.Contains(document.Error.CorrectiveAction, expected) {
							t.Fatalf("recovery altered exact command arguments: got=%s want=%s", document.Error.CorrectiveAction, expected)
						}
					}
					count := 0
					if configPath != "" {
						count = 2
					}
					if strings.Count(document.Error.CorrectiveAction, "--config") != count || strings.Contains(string(data), "secret-value") || !strings.Contains(document.Error.CorrectiveAction, output.Redacted) {
						t.Fatalf("recovery binding or redaction changed: %s", data)
					}
				})
			}
		}
	}
}

func TestRecoveryBindingPreservesNilHelpAcrossOutputModes(t *testing.T) {
	value := struct {
		Help []string `json:"help"`
	}{}
	for _, configPath := range []string{"", "selected config/settings.yaml"} {
		for _, full := range []bool{false, true} {
			options := output.Options{ConfigPath: configPath, Full: full, JSON: true, SavedResult: true}
			var out bytes.Buffer
			if err := output.RenderWithOptions(&out, value, options); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), `"help":null`) {
				t.Fatalf("recovery binding changed nil Help output: %s", out.String())
			}
			data, err := output.SnapshotWithOptions(value, 4096, options)
			if err != nil || !strings.Contains(string(data), `"help":null`) {
				t.Fatalf("recovery binding changed nil Help snapshot: data=%s err=%v", data, err)
			}
			options.JSON = false
			out.Reset()
			if err := output.RenderWithOptions(&out, value, options); err != nil || out.String() != "help: null\n" {
				t.Fatalf("recovery binding changed nil Help TOON output: data=%s err=%v", out.String(), err)
			}
		}
	}
}
