package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedRegistryIsClean(t *testing.T) {
	contract := filepath.Join("..", "..", "..", "tadx-v1-capability-contract-final.md")
	rows, err := readRows(contract)
	if err != nil {
		t.Fatalf("readRows returned error: %v", err)
	}
	if got, want := len(rows), 73; got != want {
		t.Fatalf("readRows returned %d capability rows, want %d", got, want)
	}
	generated, err := render(rows)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	path := filepath.Join("..", "registry_gen.go")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated registry: %v", err)
	}
	if string(generated) != string(committed) {
		t.Fatalf("%s is stale; run go generate ./internal/capability", path)
	}
}

func TestConvertRejectsUnsafeContractValues(t *testing.T) {
	tests := []struct {
		name   string
		column int
		value  string
		want   string
	}{
		{name: "status", column: 4, value: "Shpi", want: "unknown status"},
		{name: "operation type", column: 3, value: "Read", want: "unknown operation type"},
		{name: "local write", column: 9, value: "Maybe", want: "local write"},
		{name: "remote mutation", column: 10, value: "Yse", want: "remote mutation"},
		{name: "supports preview", column: 11, value: "", want: "supports preview"},
		{name: "evidence", column: 16, value: "Unclassified", want: "unknown evidence level"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := validContractRow()
			row[test.column] = test.value
			_, err := convert(row)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("convert() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestConvertRestrictsDelegatedBooleanExceptionsByField(t *testing.T) {
	tests := []struct {
		name    string
		column  int
		value   string
		wantErr string
	}{
		{name: "outside TADX in local write", column: 9, value: "N/A outside TADX"},
		{name: "outside TADX in remote mutation", column: 10, value: "N/A outside TADX"},
		{name: "outside TADX in supports preview", column: 11, value: "N/A outside TADX"},
		{name: "optional change set in local write", column: 9, value: "Optional change set"},
		{name: "optional change set in remote mutation", column: 10, value: "Optional change set", wantErr: "remote mutation"},
		{name: "optional change set in supports preview", column: 11, value: "Optional change set", wantErr: "supports preview"},
		{name: "reasoning stage in remote mutation", column: 10, value: "No at reasoning stage"},
		{name: "reasoning stage in supports preview", column: 11, value: "No at reasoning stage"},
		{name: "reasoning stage in local write", column: 9, value: "No at reasoning stage", wantErr: "local write"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := validContractRow()
			row[4] = "Delegated"
			row[5] = "Agent / Skill"
			row[16] = "Delegated reasoning"
			row[test.column] = test.value

			_, err := convert(row)
			if test.wantErr == "" && err != nil {
				t.Fatalf("convert() error = %v, want success", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("convert() error = %v, want error containing %q", err, test.wantErr)
			}
		})
	}
}

func TestReadRowsRejectsMalformedCapabilityRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.md")
	contents := "| Capability ID | Surface |\n| --- | --- |\n| sample.get | `tadx sample get` |\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := readRows(path)
	if err == nil || !strings.Contains(err.Error(), "17 columns") {
		t.Fatalf("readRows() error = %v, want malformed-row error", err)
	}
}

func TestReadRowsRejectsInvalidCapabilityID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.md")
	header := make([]string, 17)
	header[0] = "Capability ID"
	row := validContractRow()
	row[0] = "Bad.ID"
	contents := "| " + strings.Join(header, " | ") + " |\n| " + strings.Join(row, " | ") + " |\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := readRows(path)
	if err == nil || !strings.Contains(err.Error(), "invalid capability ID") {
		t.Fatalf("readRows() error = %v, want invalid-ID error", err)
	}
}

func TestReadRowsRejectsFormattedCapabilityID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.md")
	header := make([]string, 17)
	header[0] = "Capability ID"
	row := validContractRow()
	row[0] = "`sample.get`"
	contents := "| " + strings.Join(header, " | ") + " |\n| " + strings.Join(row, " | ") + " |\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := readRows(path)
	if err == nil || !strings.Contains(err.Error(), "invalid capability ID") {
		t.Fatalf("readRows() error = %v, want formatted-ID error", err)
	}
}

func TestReadRowsRequiresCapabilityRegistry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.md")
	if err := os.WriteFile(path, []byte("# Contract\n\nNo registry here.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := readRows(path)
	if err == nil || !strings.Contains(err.Error(), "no capability registry") {
		t.Fatalf("readRows() error = %v, want missing-registry error", err)
	}
}

func validContractRow() []string {
	return []string{
		"sample.get", "`tadx sample get`", "Inspect one sample.", "Inspect", "Ship", "CLI", "-",
		"Sample ID", "Local / all", "No", "No", "No", "Exact ID", "None", "Local sample store",
		"Test evidence", "Architecture-locked local contract",
	}
}
