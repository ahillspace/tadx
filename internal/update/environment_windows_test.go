//go:build windows

package update

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsPowerShellRebuildsItsModulePath(t *testing.T) {
	// A PowerShell 7 parent passes its own module directories through native Go.
	// Model an incompatible Core module shadowing Windows modules, without requiring pwsh.
	moduleRoot := t.TempDir()
	module := filepath.Join(moduleRoot, "Microsoft.PowerShell.Utility")
	if err := os.MkdirAll(module, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "Microsoft.PowerShell.Utility.psd1"), []byte("@{ ModuleVersion='7.0.0'; FunctionsToExport=@('Get-FileHash'); RootModule='missing.psm1' }"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PSModulePath", moduleRoot)
	t.Setenv("TADX_UPDATER_ENV_TEST", "preserved")
	hashFixture := filepath.Join(t.TempDir(), "hash-fixture")
	hashContent := []byte("tadx updater environment test")
	if err := os.WriteFile(hashFixture, hashContent, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TADX_UPDATER_HASH_TEST", hashFixture)
	command := "$ErrorActionPreference='Stop'; $hash=(Get-FileHash -Algorithm SHA256 -LiteralPath $env:TADX_UPDATER_HASH_TEST).Hash; $archive=(Get-Command Expand-Archive -ErrorAction Stop).Name; [Console]::WriteLine('{0}|{1}|{2}', $hash, $archive, $env:TADX_UPDATER_ENV_TEST)"
	// Cold Windows PowerShell module loading competes with the full package suite
	// on CI. This is a test harness bound; the real installer has five minutes.
	output, err := run(t.Context(), 30*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
	if err != nil {
		t.Fatalf("Windows PowerShell cannot load its built-in installer commands: %v: %s", err, output)
	}
	expected := fmt.Sprintf("%X|Expand-Archive|preserved", sha256.Sum256(hashContent))
	actual := strings.TrimSpace(string(output))
	parentPreserved := os.Getenv("PSModulePath") == moduleRoot
	if actual != expected || !parentPreserved {
		t.Fatalf("built-in command result = %q, want %q; parent module path preserved = %t", actual, expected, parentPreserved)
	}
}

func TestModulePathNormalizationIsWindowsPowerShellOnly(t *testing.T) {
	for _, program := range []string{"PoWeRsHeLl.ExE", "pwsh.exe", "gh.exe"} {
		cmd := &exec.Cmd{Path: program, Env: []string{"pSmOdUlEpAtH=core", "PATH=keep", "OTHER=preserved"}}
		scope, err := newProcessScope(cmd)
		if err != nil {
			t.Fatal(err)
		}
		scope.close()
		joined := strings.Join(cmd.Env, ";")
		if strings.Contains(joined, "pSmOdUlEpAtH=") == strings.EqualFold(program, "powershell.exe") {
			t.Fatalf("wrong module-path normalization for %s: %v", program, cmd.Env)
		}
		if !strings.Contains(joined, "PATH=keep") || !strings.Contains(joined, "OTHER=preserved") {
			t.Fatal(cmd.Env)
		}
	}
}
