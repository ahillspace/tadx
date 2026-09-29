package config_test

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestValidVariableReferenceAcceptsOnlyPortableNames(t *testing.T) {
	for _, reference := range []string{"A", "_", "TADX_PROD_PAT_NAME", "lower_case", "_LEADING", "X9", strings.Repeat("A", 128)} {
		if !config.ValidVariableReference(reference) {
			t.Errorf("reference %q rejected", reference)
		}
	}
	for _, reference := range []string{
		"",
		"abc123DEF==:ghiJKL456",
		"NAME=VALUE",
		"HAS SPACE",
		" LEADING_SPACE",
		"TRAILING_SPACE ",
		"9LEADING_DIGIT",
		"HYPHEN-NAME",
		"DOT.NAME",
		"SLASH/NAME",
		"PERCENT%NAME%",
		"$DOLLAR",
		"NUL\x00NAME",
		"NEW\nLINE",
		"ÜNICODE",
		strings.Repeat("A", 129),
	} {
		if config.ValidVariableReference(reference) {
			t.Errorf("reference %q accepted", reference)
		}
	}
}

func TestVariableReferenceRuleDoesNotDependOnInput(t *testing.T) {
	rule := config.VariableReferenceRule
	if rule == "" || strings.ContainsAny(rule, "=:") {
		t.Fatalf("rule = %q", rule)
	}
}
