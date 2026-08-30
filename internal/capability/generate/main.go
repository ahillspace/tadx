// Command generate builds the checked-in Go registry from the canonical V1 contract table.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var blockerPattern = regexp.MustCompile(`\b(B[1-4])\b`)
var capabilityIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*){1,2}$`)

func main() {
	contract := flag.String("contract", "../../tadx-v1-capability-contract-final.md", "path to the V1 capability contract")
	out := flag.String("out", "registry_gen.go", "generated Go file")
	flag.Parse()
	rows, err := readRows(*contract)
	if err != nil {
		panic(err)
	}
	generated, err := render(rows)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Clean(*out), generated, 0o644); err != nil {
		panic(err)
	}
}

func readRows(path string) ([][]string, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var rows [][]string
	inRegistryTable := false
	sawRegistry := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if sawRegistry && strings.HasPrefix(line, "## 3.") {
			break
		}
		if !strings.HasPrefix(line, "|") {
			inRegistryTable = false
			continue
		}
		columns := splitRow(line)
		if len(columns) > 0 && columns[0] == "Capability ID" {
			if len(columns) != 17 {
				return nil, fmt.Errorf("capability registry header has %d columns, want 17 columns", len(columns))
			}
			inRegistryTable = true
			sawRegistry = true
			continue
		}
		if !inRegistryTable {
			continue
		}
		if len(columns) != 17 {
			return nil, fmt.Errorf("capability row %q has %d columns, want 17 columns", columns[0], len(columns))
		}
		if strings.HasPrefix(columns[0], "---") {
			continue
		}
		id := plain(columns[0])
		if columns[0] != id || !capabilityIDPattern.MatchString(id) {
			return nil, fmt.Errorf("invalid capability ID %q", id)
		}
		rows = append(rows, columns)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !sawRegistry || len(rows) == 0 {
		return nil, fmt.Errorf("contract contains no capability registry rows")
	}
	return rows, nil
}

func splitRow(line string) []string {
	line = strings.TrimSpace(strings.Trim(line, "|"))
	parts := strings.Split(line, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func render(rows [][]string) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("// Code generated from tadx-v1-capability-contract-final.md; DO NOT EDIT.\n\n")
	output.WriteString("package capability\n\n")
	output.WriteString("var canonicalDefinitions = []Definition{\n")
	for _, row := range rows {
		definition, err := convert(row)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&output, "\t%s,\n", definition)
	}
	output.WriteString("}\n")
	return format.Source(output.Bytes())
}

func convert(row []string) (string, error) {
	status := plain(row[4])
	if status != "Ship" && status != "Blocked" && status != "Delegated" {
		return "", fmt.Errorf("%s: unknown status %q", row[0], status)
	}
	operationType := plain(row[3])
	if operationType != "Find" && operationType != "Inspect" && operationType != "Change" && operationType != "Deliver" {
		return "", fmt.Errorf("%s: unknown operation type %q", row[0], operationType)
	}
	owner, err := ownerConstant(plain(row[5]))
	if err != nil {
		return "", fmt.Errorf("%s: %w", row[0], err)
	}
	disposition := "DispositionShip"
	verification := "VerificationReady"
	implementation := "ImplementationPlanned"
	blocker := `BlockerID("")`
	if status == "Blocked" {
		verification = "VerificationBlocked"
		match := blockerPattern.FindStringSubmatch(plain(row[16]))
		if len(match) != 2 {
			return "", fmt.Errorf("%s: blocked row has no B1-B4 reference", row[0])
		}
		blocker = "Blocker" + match[1]
	} else if status == "Delegated" {
		disposition = "DispositionDelegated"
		implementation = "ImplementationExternalDelegated"
	}
	localWrite, err := yesNo(row[9], "local write", status, "N/A outside TADX", "Optional change set")
	if err != nil {
		return "", fmt.Errorf("%s: %w", row[0], err)
	}
	remoteMutation, err := yesNo(row[10], "remote mutation", status, "N/A outside TADX", "No at reasoning stage")
	if err != nil {
		return "", fmt.Errorf("%s: %w", row[0], err)
	}
	requiresApply, err := yesNo(row[11], "requires apply", status, "N/A outside TADX", "No at reasoning stage")
	if err != nil {
		return "", fmt.Errorf("%s: %w", row[0], err)
	}
	evidence, err := evidenceConstant(plain(row[16]), status)
	if err != nil {
		return "", fmt.Errorf("%s: %w", row[0], err)
	}
	return fmt.Sprintf("Definition{ID:%s, Surface:%s, Outcome:%s, Type:%s, Disposition:%s, Owner:%s, MCPOverlap:%s, Selectors:%s, Availability:%s, LocalWrite:%t, RemoteMutation:%t, RequiresApply:%t, SafetyGuard:%s, ArtifactEffect:%s, Upstream:%s, Evidence:%s, EvidenceLevel:%s, Verification:%s, Implementation:%s, Validation:%s, Blocker:%s, CommandPath:%s}",
		quote(row[0]), quote(plain(row[1])), quote(plain(row[2])), "Operation"+operationType, disposition, owner,
		quote(dashEmpty(plain(row[6]))), quote(plain(row[7])), quote(plain(row[8])), localWrite, remoteMutation, requiresApply,
		quote(plain(row[12])), quote(plain(row[13])), quote(plain(row[14])), quote(plain(row[15])), evidence,
		verification, implementation, quote(plain(row[16])), blocker, "nil"), nil
}

func plain(value string) string {
	replacer := strings.NewReplacer("`", "", "**", "", "Â§", "§")
	return strings.TrimSpace(replacer.Replace(value))
}

func dashEmpty(value string) string {
	if value == "—" || value == "-" {
		return ""
	}
	return value
}

func quote(value string) string { return strconv.Quote(value) }

func yesNo(value, field, status string, delegatedValues ...string) (bool, error) {
	value = plain(value)
	switch value {
	case "Yes":
		return true, nil
	case "No":
		return false, nil
	}
	if status == "Delegated" {
		for _, delegatedValue := range delegatedValues {
			if value == delegatedValue {
				return false, nil
			}
		}
	}
	return false, fmt.Errorf("%s must be Yes or No, got %q", field, value)
}

func evidenceConstant(validation, status string) (string, error) {
	lower := strings.ToLower(validation)
	switch {
	case strings.Contains(lower, "live-verified"):
		return "EvidenceLiveVerified", nil
	case strings.Contains(lower, "contract-verified"):
		return "EvidenceContractVerified", nil
	case strings.Contains(lower, "docs-only") || status == "Blocked":
		return "EvidenceDocsOnly", nil
	case strings.Contains(lower, "local contract"):
		return "EvidenceLocalContract", nil
	case strings.Contains(lower, "architecture-locked"):
		return "EvidenceArchitectureLocked", nil
	case status == "Delegated" && strings.Contains(lower, "delegated"):
		return "EvidenceArchitectureLocked", nil
	}
	return "", fmt.Errorf("unknown evidence level in %q", validation)
}

func ownerConstant(owner string) (string, error) {
	switch owner {
	case "CLI":
		return "OwnerCLI", nil
	case "MCP":
		return "OwnerMCP", nil
	case "Tableau / Desktop MCP":
		return "OwnerTableauDesktopMCP", nil
	case "Agent / Skill":
		return "OwnerAgentSkill", nil
	default:
		return "", fmt.Errorf("unknown owner %q", owner)
	}
}
