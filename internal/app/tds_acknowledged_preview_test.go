package app

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestTDSSaveCLIStalePreviewCannotAcknowledgeNewUncertainties(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	h.options = withSiteMutationConsent(t, h.options, true)
	h.ds(true, "checkout", "--id", fixtureTDSOriginal)
	save := []string{"save", "overwrite", "--work-copy", fixtureTDSCopy, "--field", "Sales"}
	preview := func(args []string) string {
		t.Helper()
		var p struct {
			Token string `json:"preview_token"`
		}
		if err := json.Unmarshal([]byte(h.ds(true, append(slices.Clone(args), "--preview")...)), &p); err != nil || p.Token == "" {
			t.Fatalf("missing preview token: %v", err)
		}
		return p.Token
	}
	original := preview(save)
	j, err := operationrun.DefaultTDSJournal()
	if err != nil {
		t.Fatal(err)
	}
	sc := value.TDSScope{Server: h.server.URL, SiteLUID: fixtureTDSSite, CallerLUID: "user-luid"}
	first, err := j.Begin(t.Context(), value.TDSAttempt{Scope: sc, Operation: "datasource.save.as-new", WorkCopyLUID: fixtureTDSCopy}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := j.Begin(t.Context(), value.TDSAttempt{Scope: sc, Operation: "datasource.save.overwrite", WorkCopyLUID: "66666666-6666-6666-6666-666666666666", SourceLUID: fixtureTDSOriginal}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ack := append(slices.Clone(save), "--acknowledge-unknown", first.ID, "--acknowledge-unknown", second.ID)
	before := h.fixture.writeCount()
	text := h.ds(false, append(slices.Clone(ack), "--expect-preview", original)...)
	requireTDSJSON(t, text, "not_attempted", "differs from its preview")
	if h.fixture.writeCount() != before {
		t.Fatal("stale approval published a work copy")
	}
	for _, id := range []string{first.ID, second.ID} {
		rows, _, err := j.Attempts(t.Context(), sc, id, "", "")
		if err != nil || len(rows) != 1 || rows[0].SupersededBy != "" {
			t.Fatalf("stale approval consumed guard %q", id)
		}
	}
	fresh := preview(ack)
	reordered := append(slices.Clone(save), "--acknowledge-unknown", second.ID, "--acknowledge-unknown", first.ID)
	requireTDSJSON(t, h.ds(true, append(reordered, "--expect-preview", fresh)...), "acknowledged", "matched")
	if h.fixture.writeCount() != before+1 {
		t.Fatal("fresh approval did not publish exactly once")
	}
}
