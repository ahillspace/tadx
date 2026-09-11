package install_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/ahillspace/tadx/actions/agent/install"
	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/output"
)

type installer struct {
	calls int
	input install.Input
}

func (i *installer) Install(_ context.Context, input install.Input) (install.Result, error) {
	i.calls++
	i.input = input
	return install.Result{Status: "preview", Skills: []install.Skill{
		{Name: "tadx", Status: "install", Path: ".codex/skills/tadx", SHA256: "bundle-tadx", Files: 4},
		{Name: "tadx-pulse", Status: "install", Path: ".codex/skills/tadx-pulse", SHA256: "bundle-pulse", Files: 2},
	}}, nil
}

func TestExecuteValidatesTargetBeforeSideEffects(t *testing.T) {
	dependency := &installer{}
	for _, target := range []string{"", "../codex", "Codex", "all", "other"} {
		if _, err := install.New(dependency).Execute(context.Background(), install.Input{Target: target}); err == nil {
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
			if _, err := install.New(dependency).Execute(context.Background(), install.Input{Target: target}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreviewProjections(t *testing.T) {
	dependency := &installer{}
	input := install.Input{Target: "codex", Preview: true, Force: true}
	result, err := install.New(dependency).Execute(context.Background(), input)
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
