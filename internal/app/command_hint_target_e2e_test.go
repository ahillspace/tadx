package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestGeneratedInspectionHintKeepsResolvedEnvironmentThroughCLI(t *testing.T) {
	const selectedEnvironment = "selected environment's $literal"
	var wrongRequests atomic.Int32
	wrong := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrongRequests.Add(1)
		http.Error(w, "wrong default environment", http.StatusNotFound)
	}))
	defer wrong.Close()
	var inspected atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/api/3.29/auth/signout":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><users/></tsResponse>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/users":
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><user id="created-user" name="author@example.test" siteRole="Creator" authSetting="ServerDefault"/></tsResponse>`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/created-user":
			inspected.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><user id="created-user" name="author@example.test" siteRole="Creator"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	configPath := filepath.Join(t.TempDir(), "config.json")
	environment := func(url string) map[string]any {
		return map[string]any{"url": url, "site_content_url": "", "api_version": "3.29", "auth": map[string]any{"type": "pat", "pat_name_env": "DIAGNOSTIC_PAT_NAME", "pat_secret_env": "DIAGNOSTIC_PAT_SECRET"}}
	}
	config, err := json.Marshal(map[string]any{"version": 1, "default_environment": "wrong", "environments": map[string]any{"wrong": environment(wrong.URL), selectedEnvironment: environment(server.URL)}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	options.ConfigPath = configPath
	var output bytes.Buffer
	args := []string{"admin", "user", "create", "--environment", selectedEnvironment, "--name", "author@example.test", "--site-role", "Creator", "--auth-setting", "ServerDefault"}
	if code := app.Run(context.Background(), args, &output, options); code != 0 {
		t.Fatalf("create exit=%d output=%s", code, output.String())
	}
	decoded, err := toon.Decode(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(decoded)
	var result struct {
		Help []string `json:"help"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil || len(result.Help) != 1 {
		t.Fatalf("help=%v err=%v", result.Help, err)
	}
	output.Reset()
	followup := hintArgumentsFromHostShell(t, result.Help[0])
	code := app.Run(context.Background(), followup, &output, options)
	if code != 0 || wrongRequests.Load() != 0 || inspected.Load() != 1 {
		t.Fatal(fmt.Sprintf("generated hint chose wrong target: hint=%q exit=%d wrong=%d inspected=%d output=%s", result.Help[0], code, wrongRequests.Load(), inspected.Load(), output.String()))
	}
}

func hintArgumentsFromHostShell(t *testing.T, hint string) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "function tadx { ConvertTo-Json -InputObject @($args) -Compress }; "+hint)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("parse generated hint: %v %s", err, output)
		}
		var args []string
		if err := json.Unmarshal(output, &args); err != nil {
			t.Fatal(err)
		}
		return args
	}
	command := exec.Command("sh", "-c", "tadx() { printf '%s\\0' \"$@\"; }; "+hint)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("parse generated hint: %v %s", err, output)
	}
	return strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00")
}
