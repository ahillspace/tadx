//go:build windows

package update

import (
	"context"
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
	output, err := run(context.Background(), 10*time.Second, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; Get-FileHash -LiteralPath ([IO.Path]::Combine($PSHOME,'powershell.exe')); Get-Command Expand-Archive | Select-Object -ExpandProperty Name; [Console]::WriteLine($env:TADX_UPDATER_ENV_TEST)")
	if err != nil {
		t.Fatalf("Windows PowerShell cannot load its built-in installer commands: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "preserved") || os.Getenv("PSModulePath") != moduleRoot {
		t.Fatal("unrelated child environment or parent environment changed")
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
