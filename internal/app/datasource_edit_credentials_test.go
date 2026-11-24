package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTDSCLIProtectsAuthenticatedEditInputs(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	h.ds(true, "checkout", "--id", fixtureTDSOriginal)
	// Fixture sign-in returns this session token and the selected PAT is read
	// from the fixture environment. These values never belong in content.
	for _, secret := range []string{"test-secret", "secret-token"} {
		text := "prefix " + secret + " suffix"
		doc := fmt.Sprintf("target:\n  server: %s\n  site_luid: %s\n  work_copy_luid: %s\nchanges:\n  - op: set-column-caption\n    field: Sales\n    caption: %q\n", h.server.URL, fixtureTDSSite, fixtureTDSCopy, text)
		if err := os.WriteFile(filepath.Join(h.ws, "protected-input.yaml"), []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
		cases := [][]string{
			{"add-tag", "--id", fixtureTDSOriginal, "--tag", text, "--preview"},
			{"remove-tag", "--id", fixtureTDSOriginal, "--tag", secret},
			{"set-field-description", "--work-copy", fixtureTDSCopy, "--field", "Sales", "--description", text},
			{"set-table-description", "--work-copy", fixtureTDSCopy, "--table", text, "--description", "Quarterly", "--preview"},
			{"elements", "--id", fixtureTDSOriginal, "--field", text},
			{"checkout", "--name", text, "--project", "Test"},
			{"work-copy", "--work-copy", fixtureTDSCopy, "--field", secret},
			{"save", "as-new", "--work-copy", fixtureTDSCopy, "--name", text},
			{"save", "as-new", "--work-copy", fixtureTDSCopy, "--name", "New", "--field", text, "--preview"},
			{"save", "as-new", "--work-copy", fixtureTDSCopy, "--name", "New", "--description", text},
			{"apply-changes", "--workspace", "work", "--from-changes", "protected-input.yaml", "--preview"},
			{"work-copy", "--workspace", "work", "--from-changes", "protected-input.yaml"},
		}
		for _, args := range cases {
			before := h.fixture.writeCount()
			cmd := h.process(append(append([]string{"content", "datasource"}, args...), "--json", "--full")...)
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			if err == nil || bytes.Contains(out.Bytes(), []byte(secret)) || bytes.Contains(stderr.Bytes(), []byte(secret)) || !strings.Contains(out.String(), "protected credential") || before != h.fixture.writeCount() {
				t.Fatalf("%s rejected safely=%t, writes before=%d after=%d", args[0], err != nil, before, h.fixture.writeCount())
			}
			last := h.command(true, "last")
			if strings.Contains(last, secret) {
				t.Fatalf("%s persisted credential-bearing input", args[0])
			}
		}
	}

	journal, err := operationrun.DefaultTDSJournal()
	if err != nil {
		t.Fatal(err)
	}
	attempts, more, err := journal.Attempts(t.Context(), value.TDSScope{Server: h.server.URL, SiteLUID: fixtureTDSSite, CallerLUID: "user-luid"}, "", "", "")
	if err != nil || more || len(attempts) != 1 || attempts[0].Operation != "datasource.checkout" {
		t.Fatalf("unexpected submission after protected input: count=%d more=%t error=%v", len(attempts), more, err)
	}
	data, err := json.Marshal(attempts)
	if err != nil || bytes.Contains(data, []byte("test-secret")) || bytes.Contains(data, []byte("secret-token")) {
		t.Fatal("private receipt contains protected data")
	}
}
