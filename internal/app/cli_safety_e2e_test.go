package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestElidedEmptyStringCannotConsumeMutationPreview(t *testing.T) {
	var requests atomic.Int32
	var writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if diagnosticSignIn(w, r) {
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/groups") {
			_, _ = w.Write([]byte(`<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><groups/></tsResponse>`))
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/groups") {
			writes.Add(1)
			_, _ = w.Write([]byte(`<tsResponse><group id="unsafe" name="--preview"/></tsResponse>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	for _, args := range [][]string{
		{"admin", "group", "create", "--environment", "test", "--name", "--preview", "--json"},
		{"--json", "admin", "group", "create", "--environment", "test", "-n", "-p"},
		{"admin", "group", "create", "--environment", "test", "--nm", "--pv", "--json"},
		{"admin", "group", "create", "--environment", "test", "-fn", "--preview", "--json"},
		{"admin", "group", "create", "--environment", "test", "--name", "--help", "--json"},
		{"admin", "group", "create", "--environment", "test", "--json", "--name", "--", "--preview"},
		{"content", "workbook", "update", "--environment", "test", "--id", "workbook-1", "--description", "--json"},
	} {
		var out bytes.Buffer
		if code := app.Run(t.Context(), args, &out, options); code == 0 || requests.Load() != 0 {
			t.Fatalf("args=%v code=%d requests=%d output=%s", args, code, requests.Load(), out.String())
		}
		var result struct {
			Error struct {
				Kind             string `json:"kind"`
				Phase            string `json:"phase"`
				Outcome          string `json:"outcome"`
				CorrectiveAction string `json:"corrective_action"`
			} `json:"error"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Error.Kind != "usage" || result.Error.Phase != "validation" || result.Error.Outcome != "not_attempted" || result.Error.CorrectiveAction == "" {
			t.Fatalf("args=%v output=%s", args, out.String())
		}
	}
	var out bytes.Buffer
	args := []string{"admin", "group", "create", "--environment", "test", "--name=--preview", "--preview", "--json"}
	if code := app.Run(t.Context(), args, &out, options); code != 0 || writes.Load() != 0 || !strings.Contains(out.String(), `"name":"--preview"`) {
		t.Fatalf("explicit flag-looking name: code=%d writes=%d output=%s", code, writes.Load(), out.String())
	}
	out.Reset()
	args = []string{"admin", "group", "create", "--environment", "test", "--name", "-", "--preview", "--json"}
	if code := app.Run(t.Context(), args, &out, options); code != 0 || writes.Load() != 0 || !strings.Contains(out.String(), `"name":"-"`) {
		t.Fatalf("single-dash name: code=%d writes=%d output=%s", code, writes.Load(), out.String())
	}
	out.Reset()
	args = []string{"admin", "group", "create", "--environment", "test", "--name=valid", "--help"}
	if code := app.Run(t.Context(), args, &out, options); code != 0 || writes.Load() != 0 || !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("help ran an action: code=%d writes=%d output=%s", code, writes.Load(), out.String())
	}
}

func TestPowerShell51ElidesEmptyNativeArgument(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell 5.1 native invocation is Windows only")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "& '"+strings.ReplaceAll(executable, "'", "''")+"' '-test.run=^TestPowerShellArgumentProbe$' 'probe' '--name' '' '--preview'")
	command.Env = append(os.Environ(), "TADX_ARGV_PROBE=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("native PowerShell probe: %v: %s", err, output)
	}
	if !strings.Contains(string(output), `TADX_ARGV=["--name","--preview"]`) {
		t.Fatalf("unexpected native argv: %s", output)
	}
}

func TestPowerShellArgumentProbe(t *testing.T) {
	if os.Getenv("TADX_ARGV_PROBE") != "1" {
		return
	}
	index := -1
	for i, arg := range os.Args {
		if arg == "probe" {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatalf("probe marker missing from %v", os.Args)
	}
	encoded, err := json.Marshal(os.Args[index+1:])
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("TADX_ARGV=%s\n", encoded)
}

func TestExplicitZeroLimitIsUsageErrorBeforeRemoteRead(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer server.Close()
	options := diagnosticOptions(t, server)
	for _, args := range [][]string{
		{"content", "workbook", "list", "--environment", "test", "--limit", "0", "--json"},
		{"admin", "group", "list", "--environment", "test", "--limit=0", "--json"},
		{"capability", "list", "--limit", "0", "--json"},
	} {
		var out bytes.Buffer
		if code := app.Run(t.Context(), args, &out, options); code == 0 || requests.Load() != 0 || !strings.Contains(out.String(), `"kind":"usage"`) {
			t.Fatalf("args=%v code=%d requests=%d output=%s", args, code, requests.Load(), out.String())
		}
	}
	var out bytes.Buffer
	if code := app.Run(t.Context(), []string{"capability", "list", "--limit", "-1", "--json"}, &out, options); code == 0 || strings.Contains(out.String(), "requires a value") {
		t.Fatalf("negative numeric value was not parsed normally: code=%d output=%s", code, out.String())
	}
	out.Reset()
	if code := app.Run(t.Context(), []string{"capability", "list", "--", "--limit", "0", "--json"}, &out, options); code == 0 || strings.Contains(out.String(), "--limit must be greater than zero") {
		t.Fatalf("delimiter lost positional semantics: code=%d output=%s", code, out.String())
	}
}

func TestPolicyArgumentsAndUnknownRootCommandUseUsageContract(t *testing.T) {
	for _, args := range [][]string{
		{"policy", "status", "unexpected", "--json"},
		{"policy", "validate", "--json"},
		{"policy", "validate", "candidate.json", "extra", "--json"},
		{"unrecognized-command", "--json"},
	} {
		var out bytes.Buffer
		code := app.Run(t.Context(), args, &out, app.Options{ConfigPath: t.TempDir() + "/missing.yaml"})
		var result struct {
			Error struct {
				Kind             string `json:"kind"`
				Phase            string `json:"phase"`
				Outcome          string `json:"outcome"`
				CorrectiveAction string `json:"corrective_action"`
			} `json:"error"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("args=%v code=%d output=%s: %v", args, code, out.String(), err)
		}
		if code == 0 || result.Error.Kind != "usage" || result.Error.Phase != "validation" || result.Error.Outcome != "not_attempted" || !strings.Contains(result.Error.CorrectiveAction, "Usage:") {
			t.Fatalf("args=%v code=%d output=%s", args, code, out.String())
		}
	}
}
