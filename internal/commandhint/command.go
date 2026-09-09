// Package commandhint renders copyable commands for the host's conventional shell.
package commandhint

import (
	"runtime"
	"strings"
)

// Shell selects a quoting convention; shell strings are not interchangeable.
type Shell string

const (
	POSIX      Shell = "posix"
	PowerShell Shell = "powershell"
)

// Command renders a TADX command for PowerShell on Windows and POSIX sh elsewhere.
func Command(args ...string) string {
	shell := POSIX
	if runtime.GOOS == "windows" {
		shell = PowerShell
	}
	return CommandFor(shell, args...)
}

// Environment preserves an available resolved target without adding a site flag.
func Environment(environment string, args ...string) string {
	if environment != "" {
		args = append(append([]string(nil), args...), "--environment", environment)
	}
	return Command(args...)
}

// Target preserves logical workspace and environment selectors on commands supporting both.
func Target(environment, workspace string, args ...string) string {
	if workspace != "" {
		args = append(append([]string(nil), args...), "--workspace", workspace)
	}
	return Environment(environment, args...)
}

// CommandFor renders a TADX command using one explicit shell convention.
func CommandFor(shell Shell, args ...string) string {
	parts := make([]string, 1, len(args)+1)
	parts[0] = "tadx"
	for _, arg := range args {
		parts = append(parts, quote(shell, arg))
	}
	return strings.Join(parts, " ")
}

func quote(shell Shell, arg string) string {
	safe := arg != ""
	for _, char := range arg {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("_./:-=", char)) {
			safe = false
			break
		}
	}
	if shell == PowerShell && strings.HasPrefix(arg, "-") && strings.Contains(arg, ".") {
		safe = false
	}
	if safe {
		return arg
	}
	if shell == PowerShell {
		literal := powershellLiteral(arg)
		if arg == "" || strings.Contains(arg, "\"") || strings.HasSuffix(arg, `\`) && strings.ContainsAny(arg, " \t\r\n") {
			legacy := legacyNativeArgument(arg)
			return "$(if ((Get-Variable PSNativeCommandArgumentPassing -ValueOnly -ErrorAction Ignore) -in @('Standard','Windows')) { " + literal + " } else { " + powershellLiteral(legacy) + " })"
		}
		return literal
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

func powershellLiteral(arg string) string { return "'" + strings.ReplaceAll(arg, "'", "''") + "'" }

// Legacy PowerShell passes embedded quotes through a native command line rather
// than preserving argv. Escape only that native layer, not the PowerShell parser.
func legacyNativeArgument(arg string) string {
	if arg == "" {
		return `""`
	}
	var result strings.Builder
	slashes := 0
	for _, char := range arg {
		if char == '\\' {
			slashes++
			continue
		}
		if char == '"' {
			result.WriteString(strings.Repeat(`\`, slashes*2+1))
		} else {
			result.WriteString(strings.Repeat(`\`, slashes))
		}
		result.WriteRune(char)
		slashes = 0
	}
	if strings.ContainsAny(arg, " \t\r\n") {
		slashes *= 2
	}
	result.WriteString(strings.Repeat(`\`, slashes))
	return result.String()
}
