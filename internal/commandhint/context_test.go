package commandhint

import (
	"strings"
	"testing"
)

func TestBindConfigPreservesSuggestionArguments(t *testing.T) {
	path := "settings/team's config.yaml"
	command := Environment("dev", "content", "workbook", "inspect", "--id", "wb-1")
	got := BindConfig("Inspect the result: "+command+"; do not repeat the write.", path)
	want := "Inspect the result: " + Command("--config", path) + strings.TrimPrefix(command, "tadx") + "; do not repeat the write."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBindConfigDoesNotRewriteArgumentsOrExistingConfig(t *testing.T) {
	for _, input := range []string{
		"No command is available.",
		"not-tadx search sales",
		Command("--config", "other.yaml", "env", "list"),
		Command("env", "list", "--config=other.yaml"),
	} {
		if got := BindConfig(input, "team.yaml"); got != input {
			t.Fatalf("rewrote %q as %q", input, got)
		}
	}
	command := Command("content", "workbook", "inspect", "--name", "tadx search --config is a title")
	got := BindConfig(command, "team.yaml")
	if got != Command("--config", "team.yaml")+strings.TrimPrefix(command, "tadx") {
		t.Fatalf("argument text was interpreted as flags: %q", got)
	}
	if got := BindConfig(got, "team.yaml"); strings.Count(got, "--config team.yaml") != 1 {
		t.Fatalf("config was duplicated: %q", got)
	}
}

func TestBindConfigStopsAtUnquotedArgumentDelimiter(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		rewritten bool
	}{
		{name: "spaces", input: "tadx search  --  --config", rewritten: true},
		{name: "tabs", input: "tadx search\t--\t--config=Revenue", rewritten: true},
		{name: "terminal delimiter", input: "tadx search --", rewritten: true},
		{name: "quoted delimiter", input: "tadx search '--' --config other.yaml"},
		{name: "real config before delimiter", input: "tadx search --config other.yaml -- --config"},
		{name: "quoted positional config after delimiter", input: "tadx search -- '--config'", rewritten: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := BindConfig(test.input, "team.yaml")
			want := test.input
			if test.rewritten {
				want = Command("--config", "team.yaml") + strings.TrimPrefix(test.input, "tadx")
			}
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
			if gotAgain := BindConfig(got, "team.yaml"); gotAgain != got {
				t.Fatalf("second bind got %q, want idempotent %q", gotAgain, got)
			}
		})
	}
}
