package doctor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	doctor "github.com/ahillspace/tadx/actions/doctor"
	render "github.com/ahillspace/tadx/internal/output"
)

type connectivityChecker struct {
	err   error
	calls int
}

func (c *connectivityChecker) CheckConnectivity(context.Context, doctor.Scope) error {
	c.calls++
	return c.err
}

type cacheChecker struct {
	state doctor.CacheState
	err   error
	calls int
}

func (c *cacheChecker) CheckCache(context.Context, doctor.Scope) (doctor.CacheState, error) {
	c.calls++
	return c.state, c.err
}

type workspaceChecker struct {
	state doctor.WorkspaceState
	err   error
	calls int
}

func (c *workspaceChecker) CheckWorkspace(context.Context, doctor.Scope) (doctor.WorkspaceState, error) {
	c.calls++
	return c.state, c.err
}

func newService(t *testing.T, credentialRef string, values map[string]string, connectivity *connectivityChecker, cache *cacheChecker, workspace *workspaceChecker) *doctor.Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("version: 1\ndefault_environment: dev\nenvironments:\n  dev:\n    url: https://example.test\n    auth:\n      type: pat\n      pat_name_env: DOCTOR_PAT_NAME\n      pat_secret_env: DOCTOR_PAT_SECRET\n      credential_ref: %s\n", credentialRef)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return doctor.New(doctor.Dependencies{
		ConfigPath:   func() string { return path },
		LookupEnv:    func(name string) (string, bool) { value, ok := values[name]; return value, ok },
		Connectivity: connectivity, Cache: cache, Workspace: workspace,
	})
}

func completePAT() map[string]string {
	return map[string]string{"DOCTOR_PAT_NAME": "private-name", "DOCTOR_PAT_SECRET": "private-secret"}
}

func TestDoctorRunsEveryIndependentCheckAndRedactsDependencyErrors(t *testing.T) {
	secret := "super-secret-pat"
	privatePath := strings.Join([]string{"private", "config.yaml"}, string(os.PathSeparator))
	connectivity := &connectivityChecker{err: errors.New(secret + " at " + privatePath)}
	cache := &cacheChecker{state: doctor.CacheState{Present: true, Complete: true, Stale: true}}
	workspace := &workspaceChecker{state: doctor.WorkspaceState{Available: false}}
	service := newService(t, "", map[string]string{"DOCTOR_PAT_NAME": "private-name"}, connectivity, cache, workspace)
	output, err := service.Execute(t.Context(), doctor.Input{Environment: "dev", Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if connectivity.calls != 0 || cache.calls != 1 || workspace.calls != 1 {
		t.Fatalf("probe calls = %d %d %d", connectivity.calls, cache.calls, workspace.calls)
	}
	if len(output.Checks) != 6 || output.Checks[0].ID != "config.valid" || output.Checks[5].ID != "logging.context" {
		t.Fatalf("checks = %#v", output.Checks)
	}
	if output.Status != doctor.StatusFail || output.Counts.Pass != 2 || output.Counts.Warn != 1 || output.Counts.Fail != 2 || output.Counts.Blocked != 1 {
		t.Fatalf("output = %#v", output)
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), privatePath) {
		t.Fatalf("output exposed sensitive context: %s", data)
	}
	service = newService(t, "", completePAT(), connectivity, cache, workspace)
	output, err = service.Execute(t.Context(), doctor.Input{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if connectivity.calls != 1 || output.Checks[2].Status != doctor.StatusFail {
		t.Fatalf("connectivity failure was not observed: calls=%d check=%+v", connectivity.calls, output.Checks[2])
	}
	data, err = json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), privatePath) {
		t.Fatalf("dependency error exposed sensitive context: %s", data)
	}
}

func TestDoctorTreatsDisabledLoggingAsPass(t *testing.T) {
	service := newService(t, "", completePAT(), &connectivityChecker{}, &cacheChecker{state: doctor.CacheState{Present: true, Complete: true}}, &workspaceChecker{state: doctor.WorkspaceState{Available: true, ManifestValid: true}})
	output, err := service.Execute(t.Context(), doctor.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if output.Status != doctor.StatusPass || output.Counts.Pass != 6 || output.Counts.Warn != 0 || output.Counts.Fail != 0 || output.Checks[5].Status != doctor.StatusPass {
		t.Fatalf("output = %#v", output)
	}
}

func TestDoctorAcceptsStoredPATWithoutEnvironmentValues(t *testing.T) {
	service := newService(t, "cred_00000000000000000000000000000000", nil, &connectivityChecker{}, &cacheChecker{state: doctor.CacheState{Present: true, Complete: true}}, &workspaceChecker{state: doctor.WorkspaceState{Available: true, ManifestValid: true}})
	output, err := service.Execute(t.Context(), doctor.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if output.Checks[1].Status != doctor.StatusPass || !strings.Contains(output.Checks[1].Summary, "does not verify its credentials") || output.Checks[1].PAT.Source != "credential_store_reference" {
		t.Fatalf("PAT check = %#v", output.Checks[1])
	}
}

func TestDoctorDoesNotInspectOrReportTableauMCP(t *testing.T) {
	service := newService(t, "", completePAT(), &connectivityChecker{}, &cacheChecker{state: doctor.CacheState{Present: true, Complete: true}}, &workspaceChecker{state: doctor.WorkspaceState{Available: true, ManifestValid: true}})
	output, err := service.Execute(t.Context(), doctor.Input{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Checks) != 6 || strings.Contains(strings.ToLower(string(data)), "mcp") {
		t.Fatalf("doctor reported MCP state: %s", data)
	}
}

func TestDoctorRejectsPathLikeScopesBeforeChecks(t *testing.T) {
	calls := 0
	service := doctor.New(doctor.Dependencies{ConfigPath: func() string { calls++; return "unused" }})
	for _, input := range []doctor.Input{{Environment: strings.Join([]string{"private", "config"}, string(os.PathSeparator))}, {Workspace: "private/../config"}} {
		if _, err := service.Execute(t.Context(), input); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
	if calls != 0 {
		t.Fatalf("configuration path calls = %d", calls)
	}
}

func TestDoctorCompactProjectionOmitsCorrectiveActions(t *testing.T) {
	output := doctor.Output{Checks: []doctor.Check{{ID: "config.valid", Status: doctor.StatusFail, Summary: "Configuration is invalid.", CorrectiveAction: "Correct the configuration."}}}
	compact := output.CompactOutput().(doctor.CompactResult)
	data, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Correct the configuration") || compact.Details != "--full" {
		t.Fatalf("compact = %s", data)
	}
}

func TestDoctorOutputGolden(t *testing.T) {
	values := completePAT()
	values["TADX_LOG_LEVEL"] = "info"
	service := newService(t, "", values, &connectivityChecker{}, &cacheChecker{state: doctor.CacheState{Present: true, Complete: true}}, &workspaceChecker{state: doctor.WorkspaceState{Available: true, ManifestValid: true}})
	output, err := service.Execute(t.Context(), doctor.Input{Environment: "dev", Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
	// Keep the original projection fixture; the app contract separately checks PAT details.
	output.Checks[1].PAT = nil
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}
