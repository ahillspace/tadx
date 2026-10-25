package live

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func recoveryFixture(t *testing.T) (string, harness.Profile, Manifest) {
	t.Helper()
	state := t.TempDir()
	profile := harness.Profile{ServerURL: "https://tableau.example.test", SiteContentURL: "qualification", Environment: "test", ActorPAT: harness.Credential{NameEnv: "TADX_GUARD_TEST_NAME", SecretEnv: "TADX_GUARD_TEST_SECRET"}, Budgets: harness.Budgets{CommandSeconds: 5}}
	t.Setenv("TADX_GUARD_TEST_NAME", "guard-fixture-name")
	t.Setenv("TADX_GUARD_TEST_SECRET", "guard-fixture-secret")
	manifest := Manifest{Version: 1, SiteID: "site-a", Profile: profile, CatalogHash: "old-catalog", FixtureSpec: "old-fixture-spec"}
	manifest.TargetFingerprint = harness.Fingerprint(profile.ServerURL, profile.SiteContentURL, manifest.SiteID)
	if err := harness.WriteJSON(filepath.Join(state, "manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	return state, profile, manifest
}
func recoveryJournal(t *testing.T, state string, m Manifest, done, pending bool) string {
	t.Helper()
	path := filepath.Join(state, "runs", "0123456789abcdef0123", "workbook.pull", "journal.json")
	journal := harness.Journal{Version: 1, RunID: "0123456789abcdef0123", Target: m.TargetFingerprint, Prefix: "tadx-live-recovery", TransferPending: pending, Entries: []harness.Entry{{Kind: "workbook", Name: "tadx-live-recovery-workbook", Done: done}}}
	if err := harness.WriteJSON(path, &journal); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGuardLocalWorkersAllowsFreshStateWithoutAuthentication(t *testing.T) {
	if err := guardLocalWorkers(t.Context(), harness.Profile{}, "candidate-must-not-run", t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
func TestGuardLocalWorkersSkipsCleanedJournalDespiteCatalogDrift(t *testing.T) {
	state, p, m := recoveryFixture(t)
	recoveryJournal(t, state, m, true, false)
	// Invalid credential references and executable prove that fully cleaned state
	// needs neither native credential resolution nor candidate execution.
	p.ActorPAT = harness.Credential{CredentialRef: "not-a-credential"}
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); err != nil {
		t.Fatal(err)
	}
}
func TestGuardLocalWorkersRejectsTransferAcceptanceBeforeExecution(t *testing.T) {
	state, p, m := recoveryFixture(t)
	path := recoveryJournal(t, state, m, false, true)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state)
	if harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("transfer guard status: %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(before) {
		t.Fatal("preauthentication guard modified the cleanup journal")
	}
}
func TestGuardLocalWorkersRejectsCrossTargetJournalEvenWhenCleaned(t *testing.T) {
	state, p, m := recoveryFixture(t)
	m.TargetFingerprint = "another-target"
	recoveryJournal(t, state, m, true, false)
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("cross-target journal guard: %v", err)
	}
}
func TestGuardLocalWorkersRejectsMissingManifestWithJournal(t *testing.T) {
	state, p, m := recoveryFixture(t)
	recoveryJournal(t, state, m, false, false)
	if err := os.Remove(filepath.Join(state, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("missing manifest guard: %v", err)
	}
}
func TestGuardLocalWorkersRejectsChangedTargetBeforeCandidate(t *testing.T) {
	state, p, _ := recoveryFixture(t)
	p.SiteContentURL = "another-site"
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); err == nil {
		t.Fatal("changed target passed local guard")
	}
}
func TestGuardLocalWorkersRejectsMalformedJournal(t *testing.T) {
	state, p, m := recoveryFixture(t)
	path := recoveryJournal(t, state, m, false, false)
	if err := os.WriteFile(path, []byte(`{"schema":"broken"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("malformed journal guard: %v", err)
	}
}
func TestGuardLocalWorkersDoesNotTreatPendingCleanedEntriesAsComplete(t *testing.T) {
	state, p, m := recoveryFixture(t)
	path := recoveryJournal(t, state, m, true, true)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var journal harness.Journal
	if err = json.Unmarshal(data, &journal); err != nil {
		t.Fatal(err)
	}
	if !journal.Entries[0].Done {
		t.Fatal("test requires completed resource entries")
	}
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("pending transfer bypassed guard: %v", err)
	}
}

func TestGuardLocalWorkersObservesSavedOperationBeforeAuthentication(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "worker-observer.go")
	program := `package main
import("fmt";"os")
func main(){if len(os.Args)<5||os.Args[1]!="job"||os.Args[2]!="inspect"||os.Args[3]!="--operation-id"||os.Args[4]!="op-a"{os.Exit(3)};fmt.Print(os.Getenv("TADX_GUARD_TEST_RESPONSE"))}`
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "worker-observer")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.CommandContext(t.Context(), "go", "build", "-o", binary, source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build local observer: %v %s", err, output)
	}
	for _, test := range []struct {
		name, response string
		wantPass       bool
	}{
		{"stopped", `{"operation_id":"op-a","tracking":{"worker_alive":false}}`, true},
		{"active", `{"operation_id":"op-a","tracking":{"worker_alive":true}}`, false},
		{"unobserved", `{"operation_id":"op-a","tracking":{}}`, false},
		{"wrong-operation", `{"operation_id":"op-other","tracking":{"worker_alive":false}}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, p, m := recoveryFixture(t)
			path := recoveryJournal(t, state, m, false, false)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var journal harness.Journal
			if err = json.Unmarshal(data, &journal); err != nil {
				t.Fatal(err)
			}
			journal.Operations = []string{"op-a"}
			if err = harness.WriteJSON(path, &journal); err != nil {
				t.Fatal(err)
			}
			t.Setenv("TADX_GUARD_TEST_RESPONSE", test.response)
			err = guardLocalWorkers(t.Context(), p, binary, state)
			if (err == nil) != test.wantPass {
				t.Fatalf("local observation %s: %v", test.name, err)
			}
			if !test.wantPass && harness.ErrorStatus(err) != harness.InconclusiveStatus {
				t.Fatalf("worker refusal must preserve inconclusive outcome: %v", err)
			}
		})
	}
}

func TestGuardLocalWorkersAllowsCompletedEvidenceAfterRootCleanup(t *testing.T) {
	state, p, m := recoveryFixture(t)
	recoveryJournal(t, state, m, true, false)
	if err := os.Remove(filepath.Join(state, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	p.ActorPAT = harness.Credential{CredentialRef: "not-a-credential"}
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); err != nil {
		t.Fatalf("completed setup evidence blocked fresh setup: %v", err)
	}
}
func TestGuardLocalWorkersRejectsMalformedEvidenceWithoutManifest(t *testing.T) {
	state, p, m := recoveryFixture(t)
	path := recoveryJournal(t, state, m, true, false)
	if err := os.Remove(filepath.Join(state, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"target_fingerprint":"old-target","entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := guardLocalWorkers(t.Context(), p, "candidate-must-not-run", state); harness.ErrorStatus(err) != harness.InconclusiveStatus {
		t.Fatalf("malformed completed evidence guard: %v", err)
	}
}
