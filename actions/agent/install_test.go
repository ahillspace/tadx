package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/actions/agent"
	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
)

type installer struct {
	calls  int
	input  agent.InstallInput
	result value.AgentGuidanceResult
	err    error
}

func (i *installer) Install(_ context.Context, target string, preview, force bool) (value.AgentGuidanceResult, error) {
	i.calls++
	i.input = agent.InstallInput{Target: target, Preview: preview, Force: force}
	if i.err != nil {
		return i.result, i.err
	}
	return value.AgentGuidanceResult{Status: "preview", Skills: []value.AgentGuidanceSkill{
		{Name: "tadx", Status: "install", Path: ".codex/skills/tadx", SHA256: "bundle-tadx", Files: 4},
		{Name: "tadx-pulse", Status: "install", Path: ".codex/skills/tadx-pulse", SHA256: "bundle-pulse", Files: 2},
	}}, nil
}

func (i *installer) Uninstall(context.Context, string, bool, bool) (value.AgentGuidanceResult, error) {
	panic("unexpected uninstall")
}

func TestInstallPartialFailureKeepsCompletedTargetsAndFieldTags(t *testing.T) {
	cause := errors.New("second target locked")
	dependency := &installer{result: value.AgentGuidanceResult{
		Targets: []string{"claude", "codex"}, Status: "partial",
		Skills: []value.AgentGuidanceSkill{
			{Target: "claude", Name: "tadx", Status: "installed", Path: ".claude/skills/tadx", SHA256: "digest", Files: 2},
			{Target: "codex", Name: "tadx", Status: "failed"},
		},
	}, err: cause}
	result, err := agent.New(dependency).Install(t.Context(), agent.InstallInput{Target: "auto"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "agent.install.failed" || !errors.Is(err, cause) {
		t.Fatalf("error=%v", err)
	}
	full, err := json.Marshal(result.FullOutput())
	if err != nil {
		t.Fatal(err)
	}
	wantFull := `{"targets":["claude","codex"],"status":"partial","target":"auto","skills":[{"target":"claude","name":"tadx","status":"installed","path":".claude/skills/tadx","sha256":"digest","files":2},{"target":"codex","name":"tadx","status":"failed","path":"","sha256":"","files":0}],"help":["tadx capability list","tadx capability get agent.install --full"]}`
	if string(full) != wantFull {
		t.Fatalf("full=%s", full)
	}
	compact, err := json.Marshal(result.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	wantCompact := `{"targets":["claude","codex"],"status":"partial","target":"auto","skills":[{"target":"claude","name":"tadx","status":"installed","path":".claude/skills/tadx"},{"target":"codex","name":"tadx","status":"failed","path":""}],"details":"--full","help":["tadx capability list","tadx capability get agent.install --full"]}`
	if string(compact) != wantCompact {
		t.Fatalf("compact=%s", compact)
	}
}

func TestInstallFailureWithoutPackageStateReturnsOnlyTheError(t *testing.T) {
	cause := errors.New("target skill directory must contain only real directories inside the user home")
	dependency := &installer{err: cause}
	result, err := agent.New(dependency).Install(t.Context(), agent.InstallInput{Target: "claude"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "agent.install.failed" || !errors.Is(err, cause) {
		t.Fatalf("error=%v", err)
	}
	if !reflect.ValueOf(result).IsZero() {
		t.Fatalf("failure returned a success-shaped result: %#v", result)
	}
}

func TestExecuteValidatesTargetBeforeSideEffects(t *testing.T) {
	dependency := &installer{}
	for _, target := range []string{"", "../codex", "Codex", "all", "other"} {
		if _, err := agent.New(dependency).Install(context.Background(), agent.InstallInput{Target: target}); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	if dependency.calls != 0 {
		t.Fatal("invalid target reached installer")
	}
}

func TestExecuteAcceptsAllSupportedTargets(t *testing.T) {
	for _, target := range agenttarget.SupportedTargets() {
		t.Run(target, func(t *testing.T) {
			dependency := &installer{}
			if _, err := agent.New(dependency).Install(context.Background(), agent.InstallInput{Target: target}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreviewProjections(t *testing.T) {
	dependency := &installer{}
	input := agent.InstallInput{Target: "codex", Preview: true, Force: true}
	result, err := agent.New(dependency).Install(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if dependency.input != input {
		t.Fatalf("installer input = %#v", dependency.input)
	}
	for _, full := range []bool{false, true} {
		var actual bytes.Buffer
		if err := output.RenderWithOptions(&actual, result, output.Options{Full: full}); err != nil {
			t.Fatal(err)
		}
		name := "testdata/preview.toon"
		if full {
			name = "testdata/preview_full.toon"
		}
		expected, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
		if !bytes.Equal(actual.Bytes(), expected) {
			t.Fatalf("%s mismatch:\n%s", name, actual.String())
		}
	}
}

func TestCompactProjectionRetainsDestinationAndBackup(t *testing.T) {
	dependency := &installer{}
	result, err := agent.New(dependency).Install(t.Context(), agent.InstallInput{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"path":".codex/skills/tadx"`)) {
		t.Fatalf("compact output omits destination: %s", data)
	}
}
