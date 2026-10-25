package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMain serves a real candidate subprocess protocol without a shell wrapper.
func TestMain(m *testing.M) {
	if os.Getenv("TADX_LIVE_TEST_CHILD") == "1" {
		if os.Getenv("TADX_LIVE_TEST_OUTPUT_MODE") == "large" {
			fmt.Fprint(os.Stdout, `{"payload":"`+strings.Repeat("x", maxCommandOutput+1024)+`"}`)
		} else {
			fmt.Fprint(os.Stdout, os.Getenv("TADX_LIVE_TEST_RESPONSE"))
		}
		fmt.Fprint(os.Stderr, os.Getenv("TADX_LIVE_TEST_STDERR"))
		code, _ := strconv.Atoi(os.Getenv("TADX_LIVE_TEST_EXIT"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func helperCase(t *testing.T, response string, exit int) *Case {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TADX_LIVE_TEST_CHILD", "1")
	t.Setenv("TADX_LIVE_TEST_RESPONSE", response)
	t.Setenv("TADX_LIVE_TEST_EXIT", strconv.Itoa(exit))
	t.Setenv("TADX_LIVE_TEST_OUTPUT_MODE", "")
	t.Setenv("TADX_LIVE_TEST_STDERR", "")
	dir := t.TempDir()
	// Generic process cases use an independently held observation session.
	// Serial refresh has separate HTTP-backed tests with real PAT sign-in envelopes.
	return &Case{Binary: binary, RunDir: dir, ConfigPath: filepath.Join(dir, "case.yaml"), Profile: Profile{Budgets: Budgets{60, 5, 60}, FixturePAT: Credential{NameEnv: "TEST_SEPARATE_VERIFIER_NAME", SecretEnv: "TEST_SEPARATE_VERIFIER_SECRET"}}}
}

func TestFailedCandidatePreservesConfirmedJobAndOperationBeforeReturning(t *testing.T) {
	c := helperCase(t, `{"error":{"id":"verification-failed"},"confirmed":{"job_id":"job-accepted","operation_id":"operation-accepted"}}`, 7)
	c.Journal = testJournal(t)
	command, err := c.Command(t.Context(), "content", "workbook", "publish")
	if err == nil || command.ExitCode != 7 {
		t.Fatal("partial candidate failure did not remain a failure")
	}
	loaded, loadErr := LoadJournal(c.Journal.path, c.Journal.Target)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(loaded.Jobs) != 1 || loaded.Jobs[0] != "job-accepted" || len(loaded.Operations) != 1 || loaded.Operations[0] != "operation-accepted" {
		t.Fatalf("confirmed acceptance identities were not durable: %#v %#v", loaded.Jobs, loaded.Operations)
	}
	if len(c.Commands) != 1 || c.Commands[0].ExitCode != 7 {
		t.Fatal("failed candidate evidence was discarded")
	}
}

func TestEarlierAcceptanceCannotResolveLaterUnknownTransfer(t *testing.T) {
	c := helperCase(t, `{"error":{"id":"transport-failed"}}`, 7)
	c.Journal = testJournal(t)
	if err := c.AcceptOperation("earlier-operation"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Command(t.Context(), "content", "workbook", "publish", "--json"); err == nil {
		t.Fatal("failed transfer passed")
	}
	j, err := LoadJournal(c.Journal.path, c.Journal.Target)
	if err != nil {
		t.Fatal(err)
	}
	if !j.TransferPending {
		t.Fatal("earlier receipt cleared a new unknown transfer")
	}
}

func TestReadonlyInspectionPreservesClosureAcrossResumeAfterLocalPurge(t *testing.T) {
	t.Setenv("LIVE_TEST_ACTOR_NAME", "actor-name")
	t.Setenv("LIVE_TEST_ACTOR_SECRET", "actor-secret")
	acceptance := `{"operation_id":"worker-one","tableau_job_id":"remote-job"}`
	inspection := `{"operation_id":"worker-one","tracking":{"worker_alive":false,"phase":"completed"}}`
	c := helperCase(t, acceptance, 0)
	journal, err := NewJournal(filepath.Join(c.RunDir, "journal.json"), "run", "target-fingerprint", "tadx-live-resume")
	if err != nil {
		t.Fatal(err)
	}
	c.Journal = journal
	if _, err = c.Run(t.Context(), "content", "datasource", "publish"); err != nil {
		t.Fatal(err)
	}
	if len(c.Journal.Jobs) != 1 || c.Journal.Jobs[0] != "remote-job" {
		t.Fatal("native operation acceptance stopped journaling its Tableau job")
	}
	t.Setenv("TADX_LIVE_TEST_RESPONSE", inspection)
	if err = c.GuardOperations(t.Context()); err != nil {
		t.Fatalf("terminal operation was not closed: %v", err)
	}
	if _, err = c.Run(t.Context(), "job", "inspect", "--operation-id", "worker-one", "--full"); err != nil {
		t.Fatal(err)
	}
	if !validOperationClosure(c.Journal.OperationClosures["worker-one"]) {
		t.Fatal("read-only inspection invalidated durable closure evidence")
	}
	if err = os.RemoveAll(filepath.Join(c.RunDir, "home")); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(c.RunDir, "commands.json")); err != nil {
		t.Fatal(err)
	}
	p, _ := testProfileConfig(t, "https://tableau.example.test", "", nil)
	resumed, err := ResumeCase(t.Context(), p, c.Binary, c.RunDir, journal.Target, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resumed.Binary = filepath.Join(c.RunDir, "purged-state-candidate.exe")
	if err = resumed.GuardOperations(t.Context()); err != nil {
		t.Fatalf("resumed closure required purged local state: %v", err)
	}
}

func TestJSONCommandRejectsTrailingOutputAndRetainsEvidence(t *testing.T) {
	c := helperCase(t, `{"status":"ok"}`+"\ntrailing diagnostic", 0)
	c.Journal = testJournal(t)
	if _, err := c.Command(t.Context(), "auth", "check", "--json"); err == nil {
		t.Fatal("trailing non-JSON output passed")
	}
	if len(c.Commands) != 1 {
		t.Fatal("malformed output evidence was lost")
	}
}

func TestCandidateEvidenceRedactsActorAndVerifierCredentials(t *testing.T) {
	c := helperCase(t, `{"message":"actor-name actor-secret fixture-name fixture-secret fixture-session"}`, 0)
	c.API = &Client{patName: "fixture-name", patSecret: "fixture-secret", token: "fixture-session"}
	c.secrets = []string{"actor-name", "actor-secret"}
	t.Setenv("TADX_LIVE_TEST_STDERR", "actor-secret fixture-session")
	command, err := c.Command(t.Context(), "auth", "status")
	if err == nil {
		t.Fatal("candidate credential leak did not fail verification")
	}
	serialized, err := json.Marshal(struct {
		Command Command
		History []Command
	}{command, c.Commands})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"actor-name", "actor-secret", "fixture-name", "fixture-secret", "fixture-session"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("credential remained in serialized evidence: %s", secret)
		}
	}
	if !strings.Contains(command.Stdout, "[REDACTED]") || !strings.Contains(command.Stderr, "[REDACTED]") {
		t.Fatal("redacted evidence did not preserve useful output")
	}
}

func TestCandidateOutputLimitFailsAndRetainsBoundedEvidence(t *testing.T) {
	c := helperCase(t, "", 0)
	t.Setenv("TADX_LIVE_TEST_OUTPUT_MODE", "large")
	command, err := c.Command(t.Context(), "content", "list")
	if err == nil {
		t.Fatal("oversized candidate output passed")
	}
	if len(command.Stdout) > maxCommandOutput || len(command.Stdout) != maxCommandOutput {
		t.Fatalf("unexpected evidence bound: %d", len(command.Stdout))
	}
	if len(c.Commands) != 1 || len(c.Commands[0].Stdout) > maxCommandOutput {
		t.Fatal("command history retained unbounded output")
	}
}
