package run_test

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

	doctorrun "github.com/ahillspace/tadx/actions/doctor/run"
	render "github.com/ahillspace/tadx/internal/output"
)

type configurationChecker struct {
	state doctorrun.ConfigurationState
	err   error
	calls int
}

func (c *configurationChecker) CheckConfiguration(context.Context, doctorrun.Scope) (doctorrun.ConfigurationState, error) {
	c.calls++
	return c.state, c.err
}

type patChecker struct {
	state doctorrun.PATState
	err   error
	calls int
}

func (c *patChecker) CheckPATReferences(context.Context, doctorrun.Scope) (doctorrun.PATState, error) {
	c.calls++
	return c.state, c.err
}

type connectivityChecker struct {
	err   error
	calls int
}

func (c *connectivityChecker) CheckConnectivity(context.Context, doctorrun.Scope) error {
	c.calls++
	return c.err
}

type cacheChecker struct {
	state doctorrun.CacheState
	err   error
	calls int
}

func (c *cacheChecker) CheckCache(context.Context, doctorrun.Scope) (doctorrun.CacheState, error) {
	c.calls++
	return c.state, c.err
}

type workspaceChecker struct {
	state doctorrun.WorkspaceState
	err   error
	calls int
}

func (c *workspaceChecker) CheckWorkspace(context.Context, doctorrun.Scope) (doctorrun.WorkspaceState, error) {
	c.calls++
	return c.state, c.err
}

type loggingChecker struct {
	state doctorrun.LoggingState
	err   error
	calls int
}

func TestDoctorReportsInvalidEntriesWithoutBlockingHealthySelection(t *testing.T) {
	for _, invalidSelection := range []bool{false, true} {
		t.Run(fmt.Sprintf("invalid_selection_%t", invalidSelection), func(t *testing.T) {
			pat := &patChecker{state: doctorrun.PATState{NameVariablePresent: true, SecretVariablePresent: true}}
			connectivity := &connectivityChecker{}
			prerequisite := ""
			if invalidSelection {
				prerequisite = "config.environment.broken"
			}
			action := doctorrun.New(doctorrun.Dependencies{
				Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true, SelectionInvalid: invalidSelection, SelectionPrerequisite: prerequisite, Findings: []doctorrun.Check{
					{ID: "config.environment.other", Status: doctorrun.StatusFail, Summary: "Environment other is invalid (url).", CorrectiveAction: "Run tadx env update other."},
					{ID: "config.environment.broken", Status: doctorrun.StatusFail, Summary: "Environment broken is invalid (pat_secret_env).", CorrectiveAction: "Run tadx env update broken or tadx env remove broken."},
				}}},
				PAT: pat, Connectivity: connectivity,
				Cache: &cacheChecker{}, Workspace: &workspaceChecker{state: doctorrun.WorkspaceState{Available: true, ManifestValid: true}}, Logging: &loggingChecker{state: doctorrun.LoggingState{Valid: true}},
			})
			out, err := action.Execute(t.Context(), doctorrun.Input{Environment: "good"})
			if err != nil || len(out.Checks) != 8 || out.Checks[0].Status != doctorrun.StatusPass || out.Checks[2].ID != "config.environment.broken" {
				t.Fatalf("entry findings changed file validity: out=%+v err=%v", out, err)
			}
			if invalidSelection && (pat.calls != 0 || connectivity.calls != 0 || out.Counts.Blocked != 4) {
				t.Fatalf("invalid selection ran dependent probes: out=%+v", out)
			}
			if invalidSelection {
				for _, check := range out.Checks[3:7] {
					if check.BlockedBy != "config.environment.broken" {
						t.Fatalf("dependent probe named an unrelated invalid entry: %+v", check)
					}
				}
			}
			if !invalidSelection && (pat.calls != 1 || connectivity.calls != 1 || out.Counts.Blocked != 0) {
				t.Fatalf("unrelated invalid entry blocked healthy probes: out=%+v", out)
			}
		})
	}
}

func (c *loggingChecker) CheckLogging(context.Context, doctorrun.Scope) (doctorrun.LoggingState, error) {
	c.calls++
	return c.state, c.err
}

func TestDoctorRunsEveryIndependentCheckAndRedactsDependencyErrors(t *testing.T) {
	secret := "super-secret-pat"
	privatePath := strings.Join([]string{"private", "config.yaml"}, string(os.PathSeparator))
	configuration := &configurationChecker{state: doctorrun.ConfigurationState{Present: true}}
	pat := &patChecker{err: errors.New(secret + " at " + privatePath)}
	connectivity := &connectivityChecker{}
	cache := &cacheChecker{state: doctorrun.CacheState{Present: true, Complete: true, Stale: true}}
	workspace := &workspaceChecker{state: doctorrun.WorkspaceState{Available: false}}
	logging := &loggingChecker{state: doctorrun.LoggingState{Valid: true}}
	action := doctorrun.New(doctorrun.Dependencies{Configuration: configuration, PAT: pat, Connectivity: connectivity, Cache: cache, Workspace: workspace, Logging: logging})

	output, err := action.Execute(t.Context(), doctorrun.Input{Environment: "dev", Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.calls != 1 || pat.calls != 1 || connectivity.calls != 0 || cache.calls != 1 || workspace.calls != 1 || logging.calls != 1 {
		t.Fatalf("check calls = %d %d %d %d %d %d", configuration.calls, pat.calls, connectivity.calls, cache.calls, workspace.calls, logging.calls)
	}
	if len(output.Checks) != 6 || output.Checks[0].ID != "config.valid" || output.Checks[5].ID != "logging.context" {
		t.Fatalf("checks = %#v", output.Checks)
	}
	if output.Status != doctorrun.StatusFail || output.Counts.Pass != 2 || output.Counts.Warn != 1 || output.Counts.Fail != 2 || output.Counts.Blocked != 1 {
		t.Fatalf("output = %#v", output)
	}
	data, marshalErr := json.Marshal(output)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(data), secret) || strings.Contains(string(data), privatePath) {
		t.Fatalf("output exposed sensitive context: %s", data)
	}
}

func TestDoctorTreatsDisabledLoggingAsPass(t *testing.T) {
	action := doctorrun.New(doctorrun.Dependencies{
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, NameVariablePresent: true, SecretVariablePresent: true}},
		Connectivity:  &connectivityChecker{},
		Cache:         &cacheChecker{state: doctorrun.CacheState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true, Enabled: false}},
	})
	output, err := action.Execute(t.Context(), doctorrun.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if output.Status != doctorrun.StatusPass || output.Counts.Pass != 6 || output.Counts.Warn != 0 || output.Counts.Fail != 0 {
		t.Fatalf("output = %#v", output)
	}
	if output.Checks[5].Status != doctorrun.StatusPass {
		t.Fatalf("logging=%#v", output.Checks[5])
	}
}

func TestDoctorAcceptsStoredPATWithoutEnvironmentValues(t *testing.T) {
	action := doctorrun.New(doctorrun.Dependencies{
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, StoredCredentialPresent: true}},
		Connectivity:  &connectivityChecker{},
		Cache:         &cacheChecker{state: doctorrun.CacheState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true}},
	})

	output, err := action.Execute(t.Context(), doctorrun.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if output.Checks[1].Status != doctorrun.StatusPass || !strings.Contains(output.Checks[1].Summary, "does not verify its credentials") {
		t.Fatalf("PAT check = %#v", output.Checks[1])
	}
}

func TestDoctorDoesNotInspectOrReportTableauMCP(t *testing.T) {
	action := doctorrun.New(doctorrun.Dependencies{
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, NameVariablePresent: true, SecretVariablePresent: true}},
		Connectivity:  &connectivityChecker{},
		Cache:         &cacheChecker{state: doctorrun.CacheState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true}},
	})
	output, err := action.Execute(t.Context(), doctorrun.Input{})
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
	configuration := &configurationChecker{}
	action := doctorrun.New(doctorrun.Dependencies{Configuration: configuration})
	for _, input := range []doctorrun.Input{{Environment: strings.Join([]string{"private", "config"}, string(os.PathSeparator))}, {Workspace: "private/../config"}} {
		if _, err := action.Execute(t.Context(), input); err == nil {
			t.Fatalf("input accepted: %#v", input)
		}
	}
	if configuration.calls != 0 {
		t.Fatalf("configuration calls = %d", configuration.calls)
	}
}

func TestDoctorCompactProjectionOmitsCorrectiveActions(t *testing.T) {
	output := doctorrun.Output{Checks: []doctorrun.Check{{ID: "config.valid", Status: doctorrun.StatusFail, Summary: "Configuration is invalid.", CorrectiveAction: "Correct the configuration."}}}
	compact := output.CompactOutput().(doctorrun.CompactResult)
	data, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Correct the configuration") || compact.Details != "--full" {
		t.Fatalf("compact = %s", data)
	}
}

func TestDoctorOutputGolden(t *testing.T) {
	action := doctorrun.New(doctorrun.Dependencies{
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, NameVariablePresent: true, SecretVariablePresent: true}},
		Connectivity:  &connectivityChecker{},
		Cache:         &cacheChecker{state: doctorrun.CacheState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true, Enabled: true}},
	})
	output, err := action.Execute(t.Context(), doctorrun.Input{Environment: "dev", Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
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
