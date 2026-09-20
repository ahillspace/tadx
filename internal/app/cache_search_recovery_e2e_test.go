package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/toon"
)

type cacheRecoveryNoNetwork struct {
	calls int
}

func (t *cacheRecoveryNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, context.Canceled
}

func TestCachedSearchUnavailableFamilyProvidesExactCacheOnlyRecovery(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	t.Setenv("TADX_DEV_PAT_SECRET", "private-pat-secret")
	configPath := filepath.Join(t.TempDir(), "configuration with spaces.yaml")
	if err := config.Save(configPath, config.Config{
		Version:            config.CurrentVersion,
		DefaultEnvironment: "dev",
		Environments: map[string]config.Environment{
			"dev": {URL: "https://example.invalid", SiteContentURL: "test-site", Auth: config.Auth{Type: config.AuthTypePAT}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := targetCacheFixture(t, configPath, func() time.Time { return now })
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{
		Environment: "dev",
		Site:        "test-site",
		Kind:        "datasource",
		LUID:        "b1354c0d-5ccc-4476-9ed7-b1544a09e0f4",
		Name:        `Boeing's "Supply" Chain Reliability`,
		Coverage:    "summary",
		ObservedAt:  now.Add(-13 * time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}

	query := `Boeing's "Supply"`
	wantCommand := commandhint.Command("--config", configPath, "search", query, "--type", "datasource", "--cache", "--environment", "dev")
	cachePath := filepath.Join(filepath.Dir(configPath), store.RelativePath())
	before, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	network := &cacheRecoveryNoNetwork{}
	var output bytes.Buffer
	code := Run(t.Context(), []string{"search", query, "--type", "content", "--cache", "--environment", "dev", "--json"}, &output, Options{
		ConfigPath: configPath,
		HTTPClient: &http.Client{Transport: network},
		Now:        func() time.Time { return now },
	})
	if code != 2 {
		t.Fatalf("exit=%d output=%s", code, output.String())
	}
	var envelope errs.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Selector != "content" || envelope.Error.Resource != "datasource,flow,project,workbook" || envelope.Error.UpstreamCause != "cache does not contain flow resources" || envelope.Error.Retryable == nil || *envelope.Error.Retryable {
		t.Fatalf("error context=%#v", envelope.Error)
	}
	if strings.Contains(output.String(), "private-pat-secret") {
		t.Fatal("cache recovery output disclosed a configured PAT secret")
	}
	text := envelope.Error.CorrectiveAction
	for _, want := range []string{
		"Requested type: content.",
		"Required types: datasource, flow, project, workbook.",
		"Available cached types: datasource (partial, stale).",
		"Missing required types: flow, project, workbook.",
		wantCommand,
		"cannot establish complete content inventory",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("recovery output missing %q:\n%s", want, text)
		}
	}
	if network.calls != 0 {
		t.Fatalf("cache-only failure made %d network requests", network.calls)
	}
	after, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("cache-only recovery diagnostics mutated the cache")
	}
	const marker = "Run this cache-only search for available observations: "
	_, renderedCommand, found := strings.Cut(envelope.Error.CorrectiveAction, marker)
	if !found || renderedCommand != wantCommand {
		t.Fatalf("rendered recovery command=%q want=%q", renderedCommand, wantCommand)
	}
	var recovered bytes.Buffer
	recoveryArgs := parseCacheRecoveryCommand(t, renderedCommand)
	recoveryNetwork := &cacheRecoveryNoNetwork{}
	if recoveredCode := Run(t.Context(), recoveryArgs, &recovered, Options{HTTPClient: &http.Client{Transport: recoveryNetwork}, Now: func() time.Time { return now }}); recoveredCode != 0 || recoveryNetwork.calls != 0 || !strings.Contains(recovered.String(), "b1354c0d-5ccc-4476-9ed7-b1544a09e0f4") {
		t.Fatalf("rendered recovery failed: args=%q exit=%d requests=%d output=%s", recoveryArgs, recoveredCode, recoveryNetwork.calls, recovered.String())
	}
}

func TestCachedSearchRecoveryRendersInTOONJSONAndFullModes(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	configPath, store := cacheRecoveryFixture(t, now)
	if _, err := store.ReplaceResourceScope(t.Context(), cache.ResourceScopeReplacement{
		Environment: "dev",
		Site:        "test-site",
		Kind:        "workbook",
		Source:      "fixture",
		GeneratedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{nil, {"--full"}, {"--json"}, {"--full", "--json"}} {
		args := []string{"search", "nothing", "--type", "content", "--cache", "--environment", "dev"}
		args = append(args, flags...)
		var output bytes.Buffer
		code := Run(t.Context(), args, &output, Options{ConfigPath: configPath, HTTPClient: &http.Client{Transport: &cacheRecoveryNoNetwork{}}, Now: func() time.Time { return now }})
		if code != 2 {
			t.Fatalf("flags=%v exit=%d output=%s", flags, code, output.String())
		}
		for _, want := range []string{"Available cached types: workbook (complete, current, empty).", "Missing required types: datasource, flow, project.", "--type workbook", "cannot establish complete content inventory"} {
			if !strings.Contains(output.String(), want) {
				t.Errorf("flags=%v missing %q: %s", flags, want, output.String())
			}
		}
	}
}

func TestCachedSearchRecoveryOmitsCommandForUnrepresentableFilters(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	_, store := cacheRecoveryFixture(t, now)
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{Environment: "dev", Site: "test-site", Kind: "datasource", LUID: "ds-1", Name: "Boeing", Owner: "owner-1", ProjectPath: "Ops", Coverage: "summary", ObservedAt: now}}); err != nil {
		t.Fatal(err)
	}
	_, err := searchaction.New(cacheGlobalSearchSource{store: store}).Execute(t.Context(), searchaction.Input{
		Terms:        "Boeing",
		Type:         "content",
		Environment:  "dev",
		Site:         "test-site",
		SiteResolved: true,
		Cache:        true,
		ProjectPath:  "Ops",
		Owner:        "owner-1",
		Limit:        7,
	})
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(structured.CorrectiveAction, "tadx search") || !strings.Contains(structured.CorrectiveAction, "filters cannot be represented safely") {
		t.Fatalf("unsafe filtered recovery: %s", structured.CorrectiveAction)
	}
}

func TestCachedSearchRecoveryReportsAllAvailableTypesDeterministically(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	configPath, store := cacheRecoveryFixture(t, now)
	if _, err := store.ReplaceResourceScope(t.Context(), cache.ResourceScopeReplacement{Environment: "dev", Site: "test-site", Kind: "datasource", Source: "fixture", GeneratedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{Environment: "dev", Site: "test-site", Kind: "workbook", LUID: "wb-1", Name: "Boeing Workbook", Coverage: "summary", ObservedAt: now}}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := Run(t.Context(), []string{"search", "Boeing", "--type", "content", "--cache", "--environment", "dev", "--limit", "7", "--json"}, &output, Options{ConfigPath: configPath, HTTPClient: &http.Client{Transport: &cacheRecoveryNoNetwork{}}, Now: func() time.Time { return now }})
	if code != 2 {
		t.Fatalf("exit=%d output=%s", code, output.String())
	}
	var envelope errs.Envelope
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	wantWorkbook := commandhint.Command("--config", configPath, "search", "Boeing", "--type", "workbook", "--cache", "--limit", "7", "--environment", "dev")
	advice := envelope.Error.CorrectiveAction
	if !strings.HasSuffix(advice, wantWorkbook) || !strings.Contains(advice, "Available cached types: datasource (complete, current, empty), workbook (partial, current).") || !strings.Contains(advice, "Missing required types: flow, project.") {
		t.Fatalf("non-deterministic recovery advice: %s", advice)
	}
}

func TestCachedSearchRecoveryProtectsLeadingFlagTerm(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		term string
		luid string
	}{
		{term: "-Boeing", luid: "ds-leading"},
		{term: "--config", luid: "ds-config"},
		{term: "--config=Revenue", luid: "ds-config-revenue"},
	} {
		t.Run(test.term, func(t *testing.T) {
			configPath, store := cacheRecoveryFixture(t, now)
			if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{Environment: "dev", Site: "test-site", Kind: "datasource", LUID: test.luid, Name: test.term + " Reliability", Coverage: "summary", ObservedAt: now}}); err != nil {
				t.Fatal(err)
			}
			initialNetwork := &cacheRecoveryNoNetwork{}
			var output bytes.Buffer
			initialArgs := []string{"search", "--type", "content", "--cache", "--environment", "dev", "--json", "--", test.term}
			if code := Run(t.Context(), initialArgs, &output, Options{ConfigPath: configPath, HTTPClient: &http.Client{Transport: initialNetwork}, Now: func() time.Time { return now }}); code != 2 {
				t.Fatalf("exit=%d output=%s", code, output.String())
			}
			if initialNetwork.calls != 0 {
				t.Fatalf("initial cache-only search made %d network requests", initialNetwork.calls)
			}
			var envelope errs.Envelope
			if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			const marker = "Run this cache-only search for available observations: "
			_, renderedCommand, found := strings.Cut(envelope.Error.CorrectiveAction, marker)
			if !found {
				t.Fatalf("missing rendered recovery command: %s", envelope.Error.CorrectiveAction)
			}
			wantCommand := commandhint.Command("--config", configPath, "search", "--type", "datasource", "--cache", "--environment", "dev", "--", test.term)
			if renderedCommand != wantCommand {
				t.Fatalf("rendered recovery command=%q want=%q", renderedCommand, wantCommand)
			}
			recoveryArgs := parseCacheRecoveryCommand(t, renderedCommand)
			recoveryNetwork := &cacheRecoveryNoNetwork{}
			var recovered bytes.Buffer
			if code := Run(t.Context(), recoveryArgs, &recovered, Options{HTTPClient: &http.Client{Transport: recoveryNetwork}, Now: func() time.Time { return now }}); code != 0 {
				t.Fatalf("recovery args=%q exit=%d output=%s", recoveryArgs, code, recovered.String())
			}
			if recoveryNetwork.calls != 0 {
				t.Fatalf("recovery made %d network requests", recoveryNetwork.calls)
			}
			decoded, err := toon.Decode(recovered.Bytes())
			if err != nil {
				t.Fatalf("decode recovery output: %v\n%s", err, recovered.String())
			}
			document, ok := decoded.(map[string]any)
			if !ok {
				t.Fatalf("recovery output type=%T want object", decoded)
			}
			items, ok := document["items"].([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("recovery items=%#v want one exact identity", document["items"])
			}
			item, ok := items[0].(map[string]any)
			if !ok || item["luid"] != test.luid {
				t.Fatalf("recovery item=%#v want luid %q", items[0], test.luid)
			}
		})
	}
}

type absentCacheRecoverySource struct{}

func (absentCacheRecoverySource) Search(context.Context, searchaction.Input) (searchaction.Result, error) {
	return searchaction.Result{}, cacheSearchScopeUnavailable{resourceType: "flow", cause: errors.New("read failed")}
}

func TestCachedSearchRecoveryRetainsGenericAdviceWithoutObservedEvidence(t *testing.T) {
	_, err := searchaction.New(absentCacheRecoverySource{}).Execute(t.Context(), searchaction.Input{Type: "content", Environment: "dev", Site: "test-site", SiteResolved: true, Cache: true, Limit: 20})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.CorrectiveAction != "Review the search source and filters, then retry." || structured.Resource != "" {
		t.Fatalf("error=%#v", structured)
	}
}

func cacheRecoveryFixture(t *testing.T, now time.Time) (string, *cache.Store) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Config{
		Version:            config.CurrentVersion,
		DefaultEnvironment: "dev",
		Environments: map[string]config.Environment{
			"dev": {URL: "https://example.invalid", SiteContentURL: "test-site", Auth: config.Auth{Type: config.AuthTypePAT}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return configPath, targetCacheFixture(t, configPath, func() time.Time { return now })
}

func TestCachedSearchRecoveryArgvHelper(t *testing.T) {
	if os.Getenv("TADX_TEST_RECOVERY_ARGV_HELPER") != "1" {
		t.Skip("child process helper")
	}
	delimiter := slices.Index(os.Args, "--")
	if delimiter < 0 {
		t.Fatal("missing test harness argument delimiter")
	}
	if err := json.NewEncoder(os.Stdout).Encode(os.Args[delimiter+1:]); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func parseCacheRecoveryCommand(t *testing.T, command string) []string {
	t.Helper()
	var executable string
	var args []string
	if runtime.GOOS == "windows" {
		argvExecutable, err := os.Executable()
		if err != nil {
			t.Fatalf("resolve native argument capture executable: %v", err)
		}
		executable = "powershell.exe"
		// Bind only the executable prefix to a native child helper. The complete
		// rendered argument tail is replayed byte-for-byte by PowerShell.
		args = []string{"-NoProfile", "-Command", `$global:PSNativeCommandArgumentPassing='Standard'; $tail = $env:TADX_TEST_RECOVERY_COMMAND.Substring(5); $prefix = '& $env:TADX_TEST_ARGV_EXE ''-test.run=^TestCachedSearchRecoveryArgvHelper$'' -- '; Invoke-Expression ($prefix + $tail)`}
		process := exec.CommandContext(t.Context(), executable, args...)
		process.Env = append(os.Environ(), "TADX_TEST_RECOVERY_COMMAND="+command, "TADX_TEST_ARGV_EXE="+argvExecutable, "TADX_TEST_RECOVERY_ARGV_HELPER=1")
		output, err := process.Output()
		if err != nil {
			t.Fatalf("parse rendered recovery command: %v", err)
		}
		var parsed []string
		if err := json.Unmarshal(output, &parsed); err != nil {
			t.Fatalf("decode rendered recovery arguments: %v output=%s", err, output)
		}
		return parsed
	} else {
		executable = "sh"
		args = []string{"-c", `tadx() { printf '%s\034' "$@"; }; eval "$TADX_TEST_RECOVERY_COMMAND"`}
	}
	process := exec.CommandContext(t.Context(), executable, args...)
	process.Env = append(os.Environ(), "TADX_TEST_RECOVERY_COMMAND="+command)
	output, err := process.Output()
	if err != nil {
		t.Fatalf("parse rendered recovery command: %v", err)
	}
	output = bytes.TrimSuffix(output, []byte{0x1c})
	parts := bytes.Split(output, []byte{0x1c})
	parsed := make([]string, len(parts))
	for index, part := range parts {
		parsed[index] = string(part)
	}
	return parsed
}
