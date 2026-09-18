package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ahillspace/tadx/internal/app"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastSaveFailureDoesNotFailCompletedReadOrClaimExpansion(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "last-result.json"), 0700); err != nil {
		t.Fatal(err)
	}
	var out, warnings bytes.Buffer
	code := app.Run(t.Context(), []string{"version", "--json"}, &out, app.Options{ConfigPath: filepath.Join(directory, "config.yaml"), Stderr: &warnings})
	if code != 0 || !json.Valid(out.Bytes()) {
		t.Fatalf("code=%d output=%s", code, out.Bytes())
	}
	var warning struct {
		Warning struct {
			Code                 string `json:"code"`
			LastPotentiallyStale bool   `json:"last_potentially_stale"`
		} `json:"warning"`
	}
	if err := json.Unmarshal(warnings.Bytes(), &warning); err != nil || warning.Warning.Code != "last_result_save_failed" || !warning.Warning.LastPotentiallyStale {
		t.Fatalf("warning=%s err=%v", warnings.Bytes(), err)
	}
}

type noLastNetwork struct{ calls int }

func (n *noLastNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.calls++
	return nil, errors.New("last must not contact Tableau")
}

func TestLastDisplaysSavedFullResultWithoutReplacingIt(t *testing.T) {
	network := &noLastNetwork{}
	opts := app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: network}}
	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := app.Run(context.Background(), args, &out, opts)
		return code, out.String()
	}
	if code, _ := run("last"); code == 0 {
		t.Fatal("missing last result succeeded")
	}
	if code, out := run("capability", "get", "workbook.publish"); code != 0 {
		t.Fatal(out)
	}
	code, first := run("last")
	if code != 0 || !strings.Contains(first, "recorded_at") || !strings.Contains(first, "workbook.publish") {
		t.Fatal(code, first)
	}
	_, second := run("last")
	if first != second {
		t.Fatal("last replaced its own receipt")
	}
	if code, full := run("last", "--full"); code != 0 || full != first {
		t.Fatalf("full changed saved result: %d %s", code, full)
	}
	if code, _ := run("capability", "get", "not-a-capability"); code == 0 {
		t.Fatal("unknown capability succeeded")
	}
	if code, failed := run("last"); code != 0 || !strings.Contains(failed, "error:") || strings.Contains(failed, "exit_code: 0") || !strings.Contains(failed, "not-a-capability") {
		t.Fatalf("failed command result lost: %d %s", code, failed)
	}
	if network.calls != 0 {
		t.Fatalf("last contacted Tableau %d times", network.calls)
	}
}

func TestLastJSONDoesNotReplayNetworkOrChangeSavedDocument(t *testing.T) {
	network := &noLastNetwork{}
	opts := app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: network}}
	var firstOutput bytes.Buffer
	if code := app.Run(context.Background(), []string{"capability", "get", "workbook.publish", "--json"}, &firstOutput, opts); code != 0 {
		t.Fatalf("seed exit=%d output=%s", code, firstOutput.String())
	}
	var firstDocument any
	if err := json.Unmarshal(firstOutput.Bytes(), &firstDocument); err != nil {
		t.Fatalf("seed output is not JSON: %v", err)
	}
	var lastOutput bytes.Buffer
	if code := app.Run(context.Background(), []string{"last", "--json"}, &lastOutput, opts); code != 0 {
		t.Fatalf("last exit=%d output=%s", code, lastOutput.String())
	}
	var lastDocument any
	if err := json.Unmarshal(lastOutput.Bytes(), &lastDocument); err != nil {
		t.Fatalf("last output is not JSON: %v", err)
	}
	if network.calls != 0 || !strings.Contains(lastOutput.String(), "workbook.publish") {
		t.Fatalf("last replayed network or lost result: calls=%d output=%s", network.calls, lastOutput.String())
	}
	if _, ok := lastDocument.(map[string]any); !ok {
		t.Fatalf("last JSON document is not an object: %#v", lastDocument)
	}
	var repeat bytes.Buffer
	if code := app.Run(context.Background(), []string{"last", "--json", "--full"}, &repeat, opts); code != 0 || repeat.String() != lastOutput.String() {
		t.Fatalf("full last changed saved document: code=%d output=%s", code, repeat.String())
	}
}
