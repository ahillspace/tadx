package app_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestRunCapabilityListRendersTOON(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "capability"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-list.toon", stdout.String())
}

func TestRunCapabilityGetRendersDetail(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "get", "capability.list"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-get.toon", stdout.String())
}

func TestCapabilityHelpDerivesFromRegistry(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--help"}, &stdout, app.Options{})
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	assertGolden(t, "testdata/capability-list-help.txt", stdout.String())
}

func TestRunUnknownCapabilityReturnsStructuredOperationError(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "get", "missing"}, &stdout, app.Options{})
	if exitCode != 1 || !strings.Contains(stdout.String(), "kind: operation") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestRunUnknownFlagReturnsStructuredUsageError(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--unknown"}, &stdout, app.Options{})
	if exitCode != 2 || !strings.Contains(stdout.String(), "kind: usage") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestMutationDiscoveryGate(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}, &stdout, app.Options{MutationsEnabled: true})
	if exitCode != 0 || !strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestMutationDiscoveryRequiresEnvironmentGate(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}, &stdout, app.Options{})
	if exitCode != 2 || !strings.Contains(stdout.String(), "kind: usage") || strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func TestDefaultDiscoveryHidesMutations(t *testing.T) {
	var stdout bytes.Buffer
	exitCode := app.Run(context.Background(), []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--limit", "100"}, &stdout, app.Options{})
	if exitCode != 0 || strings.Contains(stdout.String(), "workbook.publish") {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.TrimSuffix(string(want), "\n") && got != string(want) {
		t.Fatalf("golden mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}
