package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func TestTDSAuditRetainsHistoricalSourceAndBoundedExport(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	h.fixture.mu.Lock()
	for i := range 25 {
		name := fmt.Sprintf("Measure %02d", i)
		h.fixture.source[tdsEntryKey("Column", name)] = value.TDSElement{Name: name, Column: &value.TDSColumn{Caption: new(name)}}
	}
	h.fixture.mu.Unlock()
	receipt := h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", "first.md", "--changes-file", "edits.yaml")
	var checked struct {
		WorkCopyLUID    string                   `json:"work_copy_luid"`
		Checkout        value.TDSCheckoutContext `json:"checkout_receipt"`
		MetadataWarning string                   `json:"metadata_warning"`
		Files           struct {
			Audit   string `json:"audit_file"`
			Changes string `json:"changes_file"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(receipt), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.WorkCopyLUID != fixtureTDSCopy || checked.Checkout.SourceName != "Source" || checked.Checkout.SourceProjectLUID != fixtureTDSProject || checked.MetadataWarning == "" || checked.Files.Audit != "first.md" || checked.Files.Changes != "edits.yaml" {
		t.Fatal("incomplete checkout receipt", receipt)
	}
	for _, when := range []string{checked.Checkout.SourceObservedAt, checked.Checkout.AcknowledgedAt} {
		if _, err := time.Parse(time.RFC3339Nano, when); err != nil {
			t.Fatal(err)
		}
	}
	cmd := h.process("content", "datasource", "work-copy", "--work-copy", fixtureTDSCopy, "--workspace", "work", "--audit-file", "second.md", "--json")
	compact, err := cmd.Output()
	if err != nil {
		t.Fatalf("reinspect: %v: %s", err, compact)
	}
	var inspected struct {
		Checkout     value.TDSCheckoutContext `json:"checkout_receipt"`
		Observations []struct {
			Elements       []value.TDSElement `json:"elements"`
			DisplayOmitted int                `json:"display_omitted"`
		} `json:"observations"`
		Details string `json:"details"`
	}
	if err = json.Unmarshal(compact, &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.Checkout != checked.Checkout || len(inspected.Observations) != 3 || len(inspected.Observations[0].Elements) != 10 || inspected.Observations[0].DisplayOmitted != 16 || !strings.Contains(inspected.Details, "--full") {
		t.Fatalf("unexpected compact re-export: %s", compact)
	}
	for _, file := range []string{"first.md", "second.md"} {
		data, err := os.ReadFile(filepath.Join(h.ws, file))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Source at checkout", "Source", fixtureTDSProject, checked.Checkout.AcknowledgedAt, "Measure 24", "Observed field-description candidates", "Published datasource Metadata API coverage"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q", file, want)
			}
		}
	}
	// A different PAT for the same Tableau caller can encounter a historical
	// source name which now coincides with its credential. Do not disclose it.
	t.Setenv("TADX_FIXTURE_SECRET", "Source")
	for _, args := range [][]string{{"work-copy", "--attempts"}, {"work-copy", "--work-copy", fixtureTDSCopy}} {
		denied := h.ds(false, args...)
		if strings.Contains(denied, "Source") || !strings.Contains(denied, "protected credential material") {
			t.Fatalf("unsafe historical result %s", denied)
		}
	}
	if h.fixture.writeCount() != 1 {
		t.Fatal("re-export created another copy")
	}
}
