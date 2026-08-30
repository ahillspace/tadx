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
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "|") {
			continue
		}
		columns := splitRow(line)
		if len(columns) != 17 || columns[0] == "Capability ID" || strings.HasPrefix(columns[0], "---") {
			continue
		}
		if strings.Contains(columns[0], ".") {
			rows = append(rows, columns)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
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
	path := "nil"
	if row[0] == "capability.list" {
		implementation = "ImplementationImplemented"
		path = `[]string{"capability", "list"}`
	} else if row[0] == "capability.get" {
		implementation = "ImplementationImplemented"
		path = `[]string{"capability", "get"}`
	}
	evidence := evidenceConstant(plain(row[16]), status)
	return fmt.Sprintf("Definition{ID:%s, Surface:%s, Outcome:%s, Type:%s, Disposition:%s, Owner:%s, MCPOverlap:%s, Selectors:%s, Availability:%s, LocalWrite:%t, RemoteMutation:%t, RequiresApply:%t, SafetyGuard:%s, ArtifactEffect:%s, Upstream:%s, Evidence:%s, EvidenceLevel:%s, Verification:%s, Implementation:%s, Validation:%s, Blocker:%s, CommandPath:%s}",
		quote(row[0]), quote(plain(row[1])), quote(plain(row[2])), "Operation"+plain(row[3]), disposition, owner,
		quote(dashEmpty(plain(row[6]))), quote(plain(row[7])), quote(plain(row[8])), yes(row[9]), yes(row[10]), yes(row[11]),
		quote(plain(row[12])), quote(plain(row[13])), quote(plain(row[14])), quote(plain(row[15])), evidence,
		verification, implementation, quote(plain(row[16])), blocker, path), nil
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
func yes(value string) bool     { return plain(value) == "Yes" }

func evidenceConstant(validation, status string) string {
	lower := strings.ToLower(validation)
	switch {
	case strings.Contains(lower, "live-verified"):
		return "EvidenceLiveVerified"
	case strings.Contains(lower, "docs-only") || status == "Blocked":
		return "EvidenceDocsOnly"
	case strings.Contains(lower, "local contract"):
		return "EvidenceLocalContract"
	default:
		return "EvidenceArchitectureLocked"
	}
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
