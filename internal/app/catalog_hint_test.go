package app

import (
	"runtime"
	"strings"
	"testing"
)

func TestCatalogRecoveryKeepsQuotedResolvedEnvironment(t *testing.T) {
	command := catalogScopeRefreshCommand("workbook.list", "analyst's environment")
	quoted := "'analyst'\"'\"'s environment'"
	if runtime.GOOS == "windows" {
		quoted = "'analyst''s environment'"
	}
	if !strings.Contains(command, "--environment "+quoted) || !strings.Contains(command, "--scope workbooks") {
		t.Fatalf("unsafe or mistargeted recovery: %s", command)
	}
}
