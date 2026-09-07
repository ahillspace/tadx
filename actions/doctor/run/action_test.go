package run_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	state doctorrun.ConnectivityState
	err   error
	calls int
}

func (c *connectivityChecker) CheckConnectivity(context.Context, doctorrun.Scope) (doctorrun.ConnectivityState, error) {
	c.calls++
	return c.state, c.err
}

type catalogChecker struct {
	state doctorrun.CatalogState
	err   error
	calls int
}

func (c *catalogChecker) CheckCatalog(context.Context, doctorrun.Scope) (doctorrun.CatalogState, error) {
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

func (c *loggingChecker) CheckLogging(context.Context, doctorrun.Scope) (doctorrun.LoggingState, error) {
	c.calls++
	return c.state, c.err
}

func TestDoctorRunsEveryIndependentCheckAndRedactsDependencyErrors(t *testing.T) {
	secret := "super-secret-pat"
	privatePath := strings.Join([]string{"private", "config.yaml"}, string(os.PathSeparator))
	configuration := &configurationChecker{state: doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: true}}
	pat := &patChecker{err: errors.New(secret + " at " + privatePath)}
	connectivity := &connectivityChecker{state: doctorrun.ConnectivityState{Reachable: true, Authenticated: true}}
	catalog := &catalogChecker{state: doctorrun.CatalogState{Present: true, Complete: true, Stale: true}}
	workspace := &workspaceChecker{state: doctorrun.WorkspaceState{Selected: true, Available: false}}
	logging := &loggingChecker{state: doctorrun.LoggingState{Valid: true}}
	action := doctorrun.New(doctorrun.Dependencies{Configuration: configuration, PAT: pat, Connectivity: connectivity, Catalog: catalog, Workspace: workspace, Logging: logging})

	output, err := action.Execute(context.Background(), doctorrun.Input{Environment: "dev", Workspace: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.calls != 1 || pat.calls != 1 || connectivity.calls != 1 || catalog.calls != 1 || workspace.calls != 1 || logging.calls != 1 {
		t.Fatalf("check calls = %d %d %d %d %d %d", configuration.calls, pat.calls, connectivity.calls, catalog.calls, workspace.calls, logging.calls)
	}
	if len(output.Checks) != 6 || output.Checks[0].ID != "config.valid" || output.Checks[5].ID != "logging.context" {
		t.Fatalf("checks = %#v", output.Checks)
	}
	if output.Status != doctorrun.StatusFail || output.Counts.Pass != 3 || output.Counts.Warn != 1 || output.Counts.Fail != 2 {
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
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, NameVariablePresent: true, SecretVariablePresent: true}},
		Connectivity:  &connectivityChecker{state: doctorrun.ConnectivityState{Reachable: true, Authenticated: true}},
		Catalog:       &catalogChecker{state: doctorrun.CatalogState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Selected: true, Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true, Enabled: false}},
	})
	output, err := action.Execute(context.Background(), doctorrun.Input{})
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
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, StoredCredentialPresent: true}},
		Connectivity:  &connectivityChecker{state: doctorrun.ConnectivityState{Reachable: true, Authenticated: true}},
		Catalog:       &catalogChecker{state: doctorrun.CatalogState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Selected: true, Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true}},
	})

	output, err := action.Execute(context.Background(), doctorrun.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if output.Checks[1].Status != doctorrun.StatusPass || output.Checks[1].Summary != "A PAT is configured in the native OS credential store." {
		t.Fatalf("PAT check = %#v", output.Checks[1])
	}
}

func TestDoctorDoesNotInspectOrReportTableauMCP(t *testing.T) {
	action := doctorrun.New(doctorrun.Dependencies{
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, NameVariablePresent: true, SecretVariablePresent: true}},
		Connectivity:  &connectivityChecker{state: doctorrun.ConnectivityState{Reachable: true, Authenticated: true}},
		Catalog:       &catalogChecker{state: doctorrun.CatalogState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Selected: true, Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true}},
	})
	output, err := action.Execute(context.Background(), doctorrun.Input{})
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

func TestDoctorMissingDependenciesStillReturnEveryCheck(t *testing.T) {
	output, err := doctorrun.New(doctorrun.Dependencies{}).Execute(context.Background(), doctorrun.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Checks) != 6 || output.Counts.Fail != 5 || output.Counts.Warn != 1 {
		t.Fatalf("output = %#v", output)
	}
}

func TestDoctorRejectsPathLikeScopesBeforeChecks(t *testing.T) {
	configuration := &configurationChecker{}
	action := doctorrun.New(doctorrun.Dependencies{Configuration: configuration})
	for _, input := range []doctorrun.Input{{Environment: strings.Join([]string{"private", "config"}, string(os.PathSeparator))}, {Workspace: "private/../config"}} {
		if _, err := action.Execute(context.Background(), input); err == nil {
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
		Configuration: &configurationChecker{state: doctorrun.ConfigurationState{Present: true, Valid: true, EnvironmentResolved: true}},
		PAT:           &patChecker{state: doctorrun.PATState{ReferencesConfigured: true, NameVariablePresent: true, SecretVariablePresent: true}},
		Connectivity:  &connectivityChecker{state: doctorrun.ConnectivityState{Reachable: true, Authenticated: true}},
		Catalog:       &catalogChecker{state: doctorrun.CatalogState{Present: true, Complete: true}},
		Workspace:     &workspaceChecker{state: doctorrun.WorkspaceState{Selected: true, Available: true, ManifestValid: true}},
		Logging:       &loggingChecker{state: doctorrun.LoggingState{Valid: true, Enabled: true}},
	})
	output, err := action.Execute(context.Background(), doctorrun.Input{Environment: "dev", Workspace: "development"})
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
