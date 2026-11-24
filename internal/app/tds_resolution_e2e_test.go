package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	ds "github.com/ahillspace/tadx/actions/datasource"
)

func denyTDSProjectInventory(h *tdsWorkflowCLI) {
	h.fixture.mu.Lock()
	defer h.fixture.mu.Unlock()
	h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/projects") {
			return false
		}
		h.t.Error("exact datasource lookup requested the project inventory")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `<tsResponse><error code="403004"><summary>Forbidden</summary></error></tsResponse>`)
		return true
	}
}

func TestTDSCheckoutByIDWithoutProjectInventory(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	denyTDSProjectInventory(h)
	result := h.ds(true, "checkout", "--id", fixtureTDSOriginal, "--preview")
	var preview ds.LifecycleOutput
	if err := json.Unmarshal([]byte(result), &preview); err != nil || preview.Status != "preview" || preview.SourceLUID != fixtureTDSOriginal || preview.ProjectLUID != fixtureTDSProject || preview.Name != "Source" || h.fixture.writeCount() != 0 {
		t.Fatalf("unexpected checkout preview: %s", result)
	}
}

func TestTDSVerifySaveByIDWithoutProjectInventory(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	h.ds(true, "checkout", "--id", fixtureTDSOriginal)
	saved := h.ds(true, "save", "as-new", "--work-copy", fixtureTDSCopy, "--name", "Reviewed")
	var attempt ds.LifecycleOutput
	if err := json.Unmarshal([]byte(saved), &attempt); err != nil || attempt.AttemptID == "" {
		t.Fatalf("missing save attempt: %s", saved)
	}

	// Selection by name still checks the exact canonical project path.
	requireTDSJSON(t, h.ds(true, "checkout", "--name", "Reviewed", "--project", "Test", "--preview"), fixtureTDSPublished, fixtureTDSProject)
	h.ds(false, "checkout", "--name", "Reviewed", "--project", "Other", "--preview")
	denyTDSProjectInventory(h)
	before := h.fixture.writeCount()
	verify := []string{"work-copy", "--attempt", attempt.AttemptID, "--verify"}
	result := h.ds(true, verify...)
	var verified ds.LifecycleOutput
	if err := json.Unmarshal([]byte(result), &verified); err != nil || verified.Verification != "matched" || verified.PublishedLUID != fixtureTDSPublished || verified.PublishedProjectLUID != fixtureTDSProject || verified.PublishedName != "Reviewed" {
		t.Fatalf("unexpected published verification: %s", result)
	}

	h.fixture.mu.Lock()
	h.fixture.name = "Renamed"
	h.fixture.mu.Unlock()
	requireTDSJSON(t, h.ds(false, verify...), `"verification":"mismatch"`, `"published_name":"Renamed"`)
	if h.fixture.writeCount() != before {
		t.Fatal("published verification submitted a write")
	}
}

func TestTDSCheckoutCanonicalUUID(t *testing.T) {
	const canonical = "abcdef12-1234-1234-abcd-123456789abc"
	h := newTDSWorkflowCLI(t)
	reported := canonical
	h.fixture.mu.Lock()
	h.fixture.auditProbe = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || !strings.EqualFold(r.URL.Path, "/api/3.30/sites/"+fixtureTDSSite+"/datasources/"+canonical) {
			return false
		}
		fmt.Fprintf(w, `<tsResponse><datasource id="%s" name="Source"><project id="%s" name="Test"/></datasource></tsResponse>`, reported, fixtureTDSProject)
		return true
	}
	h.fixture.mu.Unlock()
	preview := []string{"checkout", "--id", strings.ToUpper(canonical), "--preview"}
	requireTDSJSON(t, h.ds(true, preview...), `"source_luid":"`+canonical+`"`)
	h.fixture.mu.Lock()
	reported = fixtureTDSOriginal
	h.fixture.mu.Unlock()
	requireTDSJSON(t, h.ds(false, preview...), `"outcome":"not_attempted"`, "returned LUID")
	if h.fixture.writeCount() != 0 {
		t.Fatal("checkout identity preview submitted a write")
	}
}
