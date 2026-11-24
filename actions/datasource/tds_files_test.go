package datasource

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

const editDocumentPrefix = "target:\n  server: https://tableau.example.test\n  site_luid: 00000000-0000-4000-8000-000000000001\n  work_copy_luid: 00000000-0000-4000-8000-000000000002\n"
const editBody = "changes:\n  - op: set-field-description\n    field: '[Sales]'\n    description: 'Total sales'\n"

func TestTDSChangesStrict(t *testing.T) {
	good := editDocumentPrefix + editBody
	for _, bad := range []string{
		good + "---\n{}\n", strings.Replace(good, "'[Sales]'", "'*alias'\n    field: again", 1),
		strings.Replace(good, "field: '[Sales]'", "field: &column Sales\n    caption: *column", 1),
		strings.Replace(good, "field: '[Sales]'", "field: !!str Sales", 1),
		strings.Replace(good, "description: 'Total sales'", "description: 123", 1),
		strings.Replace(good, "description: 'Total sales'", "description: null", 1),
		strings.Replace(good, "description:", "extra:", 1),
		strings.Replace(good, "set-field-description", "set-price", 1),
		good + "    caption: Other\n", strings.Replace(good, "server: https://", "server: https://user:pass@", 1),
		good + "target: {}\n", strings.Replace(good, "site_luid: 0", "site_luid: invalid0", 1),
		editDocumentPrefix + "changes: []\n", good + strings.Repeat(" ", maxEditBytes),
		strings.Replace(good, "changes:\n", "changes: &batch\n", 1),
	} {
		if _, err := parseEditDocument([]byte(bad)); err == nil {
			t.Fatal("accepted malformed edit document")
		}
	}
	doc, err := parseEditDocument([]byte(good))
	if err != nil || len(doc.Changes) != 1 || doc.Changes[0].Description != "Total sales" {
		t.Fatalf("%#v %v", doc, err)
	}
	if err = doc.Target.Match(editTarget); err != nil {
		t.Fatal(err)
	}
	doc.Target.DatasourceLUID = "00000000-0000-4000-8000-000000000003"
	if err = doc.Target.Match(editTarget); err == nil {
		t.Fatal("unproven origin accepted")
	}
}

func TestTDSChangesRejectConflictingTargets(t *testing.T) {
	f := EditChange{Op: "set-field-description", Field: "Sales", Description: "A"}
	d := EditChange{Op: "delete-column", Field: "Sales"}
	for _, changes := range [][]EditChange{{f, f}, {f, d}, {d, f}, {f, {Op: "remove-field-description", Field: "Sales"}}} {
		if err := ValidateEditChanges(changes); err == nil {
			t.Fatal("conflicting plan accepted")
		}
	}
	if err := ValidateEditChanges([]EditChange{f, {Op: "set-column-role", Field: "Sales", Role: "MEASURE"}, {Op: "set-column-caption", Field: "Sales", Caption: "Total"}, {Op: "set-table-description", Table: "Sales", Description: "Summary"}}); err != nil {
		t.Fatal(err)
	}
}

func TestTDSWorkspaceExportAndReopen(t *testing.T) {
	root := t.TempDir()
	names := []string{"[Sales]", "Date\nchanges:\n  - op: edit"}
	inspect := CopyInspection{EditTarget: editTarget, TargetKind: "workcopy", Observations: []EditObservation{{ElementType: "Column", Elements: []value.TDSElement{{Name: names[0], Column: &value.TDSColumn{}}, {Name: names[1], Column: &value.TDSColumn{}}}}}}
	inspect.Observations = append(inspect.Observations, EditObservation{ElementType: "FieldDescription", Complete: true})
	if err := ValidateEditExports(root, "audit.md", "changes.yaml"); err != nil {
		t.Fatal(err)
	}
	exported, err := ExportEditCopy(root, "audit.md", "changes.yaml", inspect, nil, identityRedactor)
	if err != nil || exported.AuditFile != "audit.md" || exported.ChangesFile != "changes.yaml" {
		t.Fatal(exported, err)
	}
	data, err := os.ReadFile(filepath.Join(root, exported.ChangesFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEditDocument(root, "changes.yaml"); err == nil {
		t.Fatal("starter unexpectedly active")
	}
	text := string(data)
	idx := strings.Index(text, "# changes:")
	if idx < 0 {
		t.Fatal(text)
	}
	text = strings.Split(text, "changes: []")[0] + strings.TrimPrefix(strings.ReplaceAll(text[idx:], "\n# ", "\n"), "# ")
	if err = os.WriteFile(filepath.Join(root, "activated.yaml"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadEditDocument(root, "activated.yaml")
	if err != nil || len(doc.Changes) != 2 || doc.Target.WorkCopyLUID != editTarget.WorkCopyLUID || doc.Target.DatasourceLUID != "" {
		t.Fatalf("%#v %v", doc, err)
	}
	if err = ValidateEditExports(root, "again.md", "changes.yaml"); err == nil {
		t.Fatal("existing output accepted")
	}
	partial, err := ExportEditCopy(root, "again.md", "changes.yaml", inspect, nil, identityRedactor)
	if err == nil || partial.AuditFile != "again.md" || partial.ChangesFile != "" {
		t.Fatal(partial, err)
	}
	check, _ := os.ReadFile(filepath.Join(root, "changes.yaml"))
	if string(check) != string(data) {
		t.Fatal("overwrote conflicting input")
	}
	for _, unsafe := range []string{"../outside", "/absolute", "a/../b", "C:/external", ".git/input", "bad\\name", "a/./b"} {
		if _, err = LoadEditDocument(root, unsafe); err == nil {
			t.Fatalf("read unsafe %q", unsafe)
		}
		if err = ValidateEditExports(root, unsafe, ""); err == nil {
			t.Fatalf("export unsafe %q", unsafe)
		}
	}
	if err := os.Link(filepath.Join(root, "activated.yaml"), filepath.Join(root, "alias.yaml")); err == nil {
		if _, err = LoadEditDocument(root, "alias.yaml"); err == nil {
			t.Fatal("hard-linked input accepted")
		}
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(root, "alias")); err == nil {
		if err = ValidateEditExports(root, "alias/report.md", ""); err == nil {
			t.Fatal("symlink directory accepted")
		}
	}
}

func TestTDSAtomicExportCollision(t *testing.T) {
	root := t.TempDir()
	errs := make(chan error, 2)
	var group sync.WaitGroup
	for _, body := range []string{"one", "two"} {
		group.Go(func() { errs <- createEditFile(root, "output", []byte(body)) })
	}
	group.Wait()
	close(errs)
	var ok, exists int
	for err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, os.ErrExist):
			exists++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || exists != 1 {
		t.Fatal(ok, exists)
	}
}

func TestTDSExplicitTemplateFieldIncomplete(t *testing.T) {
	root := t.TempDir()
	inspect := CopyInspection{EditTarget: editTarget}
	out, err := ExportEditCopy(root, "audit.md", "changes.yaml", inspect, []string{"absent"}, identityRedactor)
	if err == nil || out.AuditFile != "audit.md" || out.ChangesFile != "" {
		t.Fatal(out, err)
	}
}

func identityRedactor(v string) string { return v }
