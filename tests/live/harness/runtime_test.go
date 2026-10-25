package harness

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestIndependentVerifierRequiresDistinctResolvedPATs(t *testing.T) {
	t.Setenv("LIVE_TEST_ACTOR_NAME", "actor-name")
	t.Setenv("LIVE_TEST_ACTOR_SECRET", "actor-secret")
	t.Setenv("LIVE_TEST_FIXTURE_NAME", "fixture-name")
	t.Setenv("LIVE_TEST_FIXTURE_SECRET", "fixture-secret")
	p, _ := testProfileConfig(t, "https://tableau.example.test", "", new(true))
	if err := VerifyIndependentCredentials(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	serial := p
	serial.FixturePAT = Credential{}
	if err := VerifyIndependentCredentials(t.Context(), serial); err != nil {
		t.Fatalf("serial single-PAT mode rejected: %v", err)
	}
	for _, mode := range []string{"same-name", "same-secret"} {
		t.Run(mode, func(t *testing.T) {
			candidate := p
			switch mode {
			case "same-name":
				t.Setenv("LIVE_TEST_FIXTURE_NAME", "actor-name")
			case "same-secret":
				t.Setenv("LIVE_TEST_FIXTURE_SECRET", "actor-secret")
			}
			if err := VerifyIndependentCredentials(t.Context(), candidate); ErrorStatus(err) != BlockedStatus {
				t.Fatalf("non-independent credential accepted: %v", err)
			}
		})
	}
}

func TestCaseConfigPreservesCanonicalTargetAndExistingConsentOnly(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			p, dir := testProfileConfig(t, "https://TABLEAU.example.test:443/", "", new(enabled))
			before, err := os.ReadFile(p.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			p.ServerURL = "https://tableau.example.test"
			c := &Case{Profile: p, Environment: p.Environment, ConfigPath: filepath.Join(dir, "case.yaml")}
			if err := c.makeConfig(); err != nil {
				t.Fatalf("canonical target changed during case preparation: %v", err)
			}
			copy, err := config.Load(c.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			environment, err := copy.ResolveEnvironment(p.Environment)
			if err != nil {
				t.Fatal(err)
			}
			setting, err := copy.MutationSetting(environment)
			if err != nil || setting == nil || *setting != enabled {
				t.Fatalf("isolated case changed consent: %v %v", setting, err)
			}
			after, err := os.ReadFile(p.ConfigPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("case preparation changed source configuration")
			}
			c.Profile.SiteContentURL = "different-site"
			if err := c.makeConfig(); err == nil {
				t.Fatal("case accepted a different exact site")
			}
		})
	}
}

func TestPreflightHonorsRealCandidatePolicyAndNeverEnablesConsent(t *testing.T) {
	p, dir := testProfileConfig(t, "https://tableau.example.test", "", nil)
	candidate := helperCase(t, `{"state":"unmanaged","remote_mutations":true}`, 0)
	before, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Preflight(t.Context(), p, candidate.Binary, dir, nil); ErrorStatus(err) != BlockedStatus {
		t.Fatalf("preflight authorized absent source consent: %v", err)
	}
	after, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("preflight enabled or rewrote source consent")
	}
}

func TestCasePreparationDoesNotCreateAbsentSiteConsent(t *testing.T) {
	p, dir := testProfileConfig(t, "https://tableau.example.test", "", nil)
	c := &Case{Profile: p, Environment: p.Environment, ConfigPath: filepath.Join(dir, "case.yaml")}
	if err := c.makeConfig(); err != nil {
		t.Fatal(err)
	}
	copy, err := config.Load(c.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := copy.ResolveEnvironment(p.Environment)
	if err != nil {
		t.Fatal(err)
	}
	setting, err := copy.MutationSetting(environment)
	if err != nil || setting != nil {
		t.Fatalf("case created persisted consent absent from source: %v %v", setting, err)
	}
}

func TestBoundIndependentWritesRecheckCandidatePolicyBeforeEachMutation(t *testing.T) {
	p, _ := testProfileConfig(t, "https://tableau.example.test", "", new(true))
	c := helperCase(t, `{"state":"unmanaged","remote_mutations":true}`, 0)
	c.Profile = p
	c.RequiredCapabilities = []string{"admin.group.create"}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"group":{"id":"new-group","name":"fixture"}}`)
	}))
	defer server.Close()
	c.BindAPI(observationClient(server))
	if _, err := c.API.Create(t.Context(), "group", map[string]any{"name": "fixture"}); err != nil {
		t.Fatalf("initial authorized fixture write failed: %v", err)
	}
	t.Setenv("TADX_LIVE_TEST_RESPONSE", `{"state":"unmanaged","remote_mutations":false}`)
	if _, err := c.API.Create(t.Context(), "group", map[string]any{"name": "second"}); ErrorStatus(err) != BlockedStatus {
		t.Fatalf("revoked managed mutation policy did not block independent write: %v", err)
	}
	if requests != 1 {
		t.Fatal("revoked fixture write reached Tableau")
	}
}

func TestResumeCaseAppendsCommandHistoryAndRejectsMalformedHistory(t *testing.T) {
	t.Setenv("LIVE_TEST_ACTOR_NAME", "actor-name")
	t.Setenv("LIVE_TEST_ACTOR_SECRET", "actor-secret")
	p, _ := testProfileConfig(t, "https://tableau.example.test", "", nil)
	c := helperCase(t, `{"status":"before-recovery"}`, 0)
	journal, err := NewJournal(filepath.Join(c.RunDir, "journal.json"), "run", "target-fingerprint", "tadx-live-resume")
	if err != nil {
		t.Fatal(err)
	}
	c.Journal = journal
	if _, err = c.Command(t.Context(), "before-recovery"); err != nil {
		t.Fatal(err)
	}

	resumed, err := ResumeCase(t.Context(), p, c.Binary, c.RunDir, journal.Target, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed.Commands) != 1 || resumed.Commands[0].Args[0] != "before-recovery" {
		t.Fatal("recovery discarded existing command evidence")
	}
	t.Setenv("TADX_LIVE_TEST_RESPONSE", `{"status":"after-recovery"}`)
	if _, err = resumed.Command(t.Context(), "after-recovery"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(c.RunDir, "commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	var history []Command
	if err = json.Unmarshal(data, &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Args[0] != "before-recovery" || history[1].Args[0] != "after-recovery" {
		t.Fatal("recovery did not append to persisted command history")
	}

	if err = os.WriteFile(filepath.Join(c.RunDir, "commands.json"), []byte("not valid history"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = ResumeCase(t.Context(), p, c.Binary, c.RunDir, journal.Target, nil, nil); err == nil {
		t.Fatal("recovery silently discarded malformed command history")
	}
}

func TestResumeCaseRecoversOnlyDurableExactTerminalHistory(t *testing.T) {
	t.Setenv("LIVE_TEST_ACTOR_NAME", "actor-name")
	t.Setenv("LIVE_TEST_ACTOR_SECRET", "actor-secret")
	p, _ := testProfileConfig(t, "https://tableau.example.test", "", nil)
	terminal := map[string]any{"operation_id": "worker-one", "tracking": map[string]any{"worker_alive": false, "phase": "completed"}}
	alive := map[string]any{"operation_id": "worker-one", "tracking": map[string]any{"worker_alive": true, "phase": "running"}}
	for _, test := range []struct {
		name     string
		observes []Command
		want     bool
	}{
		{name: "exact terminal proof survives later not found", observes: []Command{
			{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 0, Value: terminal},
			{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 1, Value: map[string]any{"error": map[string]any{"upstream_cause": "operation run not found"}}},
		}, want: true},
		{name: "wrong returned identity", observes: []Command{{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 0, Value: map[string]any{"operation_id": "other", "tracking": map[string]any{"worker_alive": false, "phase": "completed"}}}}},
		{name: "worker alive", observes: []Command{{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 0, Value: alive}}},
		{name: "worker observation missing", observes: []Command{{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 0, Value: map[string]any{"operation_id": "worker-one", "tracking": map[string]any{"phase": "completed"}}}}},
		{name: "nonterminal phase", observes: []Command{{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 0, Value: map[string]any{"operation_id": "worker-one", "tracking": map[string]any{"worker_alive": false, "phase": "running"}}}}},
		{name: "new acceptance after proof", observes: []Command{
			{Args: []string{"job", "inspect", "--operation-id", "worker-one", "--full"}, ExitCode: 0, Value: terminal},
			{Args: []string{"content", "datasource", "publish"}, ExitCode: 0, Value: map[string]any{"operation_id": "worker-one"}},
		}},
		{name: "inspect without full evidence", observes: []Command{{Args: []string{"job", "inspect", "--operation-id", "worker-one"}, ExitCode: 0, Value: terminal}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			journal, err := NewJournal(filepath.Join(dir, "journal.json"), "run", "target-fingerprint", "tadx-live-history")
			if err != nil {
				t.Fatal(err)
			}
			if err = (&Case{Journal: journal}).AcceptOperation("worker-one"); err != nil {
				t.Fatal(err)
			}
			if err = journal.closeOperation("worker-one", "completed", false); err != nil {
				t.Fatal(err)
			}
			commands := []Command{{Args: []string{"content", "datasource", "publish"}, ExitCode: 0, Value: map[string]any{"operation_id": "worker-one"}}}
			commands = append(commands, test.observes...)
			if err = WriteJSON(filepath.Join(dir, "commands.json"), commands); err != nil {
				t.Fatal(err)
			}
			resumed, err := ResumeCase(t.Context(), p, "unused-candidate.exe", dir, journal.Target, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			closure, exists := resumed.Journal.OperationClosures["worker-one"]
			if exists != test.want {
				t.Fatalf("closure existence = %v, want %v", exists, test.want)
			}
			if test.want && (!validOperationClosure(closure) || closure.EvidenceSource != "command-history" || closure.EvidenceCommand != 2 || !closure.ObservedAt.IsZero()) {
				t.Fatalf("historical proof source was not preserved accurately: %#v", closure)
			}
		})
	}
}
