package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

const authoredSlots = `<p><strong id="sourceTotal">0</strong> commands</p>` +
	`<span id="implementedCount">0</span><span id="plannedCount">0</span><span id="blockedCount">0</span>` +
	`<span id="totalCount">0</span><span id="remoteMutationCount">0</span><span id="footerTotal">0</span>`

func TestUpdateMapPreservesAuthoredPage(t *testing.T) {
	const prefix = "<!doctype html>\r\n<style>authored</style>\r\n" + authoredSlots + "\r\n<script id=\"capability-data\" type=\"application/json\">"
	const suffix = "</script>\r\n<script>authoredBehavior()</script>"
	path := filepath.Join(t.TempDir(), "map.html")
	if err := os.WriteFile(path, []byte(prefix+"\n[{}]\n"+suffix), 0o600); err != nil {
		t.Fatal(err)
	}
	data := []byte("[{\"id\":\"catalog.audit\"}]\n")
	counts := []mapCount{
		{"sourceTotal", 12}, {"totalCount", 12}, {"footerTotal", 12},
		{"implementedCount", 9}, {"plannedCount", 2}, {"blockedCount", 1}, {"remoteMutationCount", 4},
	}
	wantPrefix := "<!doctype html>\r\n<style>authored</style>\r\n" +
		`<p><strong id="sourceTotal">12</strong> commands</p>` +
		`<span id="implementedCount">9</span><span id="plannedCount">2</span><span id="blockedCount">1</span>` +
		`<span id="totalCount">12</span><span id="remoteMutationCount">4</span><span id="footerTotal">12</span>` +
		"\r\n<script id=\"capability-data\" type=\"application/json\">"
	for range 2 {
		if err := updateMap(path, data, counts); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := wantPrefix + "\n" + strings.TrimSpace(string(data)) + "\n" + suffix
		if string(got) != want {
			t.Fatalf("page outside snapshot and totals changed or update was not idempotent: %s", got)
		}
	}
}

func TestUpdateMapRejectsMalformedPageWithoutWriting(t *testing.T) {
	const marker = `<script id="capability-data" type="application/json">`
	counts := commandCounts(nil)
	missingSlot := strings.Replace(authoredSlots, `<span id="footerTotal">0</span>`, "", 1)
	duplicateSlot := authoredSlots + `<span id="plannedCount">0</span>`
	nonNumericSlot := strings.Replace(authoredSlots, `<span id="blockedCount">0</span>`, `<span id="blockedCount">none</span>`, 1)
	emptySlot := strings.Replace(authoredSlots, `<span id="footerTotal">0</span>`, `<span id="footerTotal"></span>`, 1)
	slotAfterMarker := missingSlot + marker + "[]</script>" + `<span id="footerTotal">0</span>`
	dataAttributeSlot := strings.Replace(authoredSlots, `<span id="footerTotal">`, `<span data-id="footerTotal">`, 1)
	for _, content := range []string{
		"<html></html>",
		authoredSlots + marker + "[]",
		authoredSlots + marker + "[]</script>" + marker + "[]</script>",
		missingSlot + marker + "[]</script>",
		duplicateSlot + marker + "[]</script>",
		nonNumericSlot + marker + "[]</script>",
		emptySlot + marker + "[]</script>",
		slotAfterMarker,
		dataAttributeSlot + marker + "[]</script>",
	} {
		path := filepath.Join(t.TempDir(), "map.html")
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := updateMap(path, []byte("[]"), counts); err == nil {
			t.Fatalf("expected malformed page rejection: %q", content)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != content {
			t.Fatalf("malformed page changed: %q, %v", got, err)
		}
	}
}

func TestCommandCountsExcludeDelegated(t *testing.T) {
	definitions := []capability.Definition{
		{ID: "cli.implemented", Owner: capability.OwnerCLI, Disposition: capability.DispositionShip, Implementation: capability.ImplementationImplemented, Verification: capability.VerificationReady},
		{ID: "cli.planned", Owner: capability.OwnerCLI, Disposition: capability.DispositionShip, Implementation: capability.ImplementationPlanned, Verification: capability.VerificationReady},
		{ID: "cli.blocked", Owner: capability.OwnerCLI, Disposition: capability.DispositionShip, Implementation: capability.ImplementationPlanned, Verification: capability.VerificationBlocked, Blocker: capability.BlockerB1},
		{ID: "cli.remote", Owner: capability.OwnerCLI, Disposition: capability.DispositionShip, Implementation: capability.ImplementationImplemented, Verification: capability.VerificationReady, RemoteMutation: true, SupportsPreview: true},
		{ID: "mcp.delegated", Owner: capability.OwnerMCP, Disposition: capability.DispositionDelegated, Implementation: capability.ImplementationExternalDelegated, Verification: capability.VerificationReady, RemoteMutation: true, SupportsPreview: true},
	}
	want := []mapCount{
		{"sourceTotal", 4}, {"totalCount", 4}, {"footerTotal", 4},
		{"implementedCount", 2}, {"plannedCount", 2}, {"blockedCount", 1}, {"remoteMutationCount", 1},
	}
	if got := commandCounts(definitions); !reflect.DeepEqual(got, want) {
		t.Fatalf("commandCounts() = %+v, want %+v", got, want)
	}
}
