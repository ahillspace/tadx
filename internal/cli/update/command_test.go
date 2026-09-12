package update

import (
	"context"
	action "github.com/ahillspace/tadx/actions/update"
	"testing"
)

type testUpdater struct {
	input action.Input
	calls int
}

func (u *testUpdater) Execute(_ context.Context, in action.Input) (action.Output, error) {
	u.calls++
	u.input = in
	return action.Output{Status: "checked"}, nil
}

type testRenderer struct{}

func (testRenderer) Render(any) error { return nil }
func TestCheckAndRepeatableTargets(t *testing.T) {
	u := &testUpdater{}
	cmd := New(Dependencies{Updater: u, Renderer: testRenderer{}})
	cmd.SetArgs([]string{"--check", "--target", "codex", "--target", "claude"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if u.calls != 1 || !u.input.Check || len(u.input.Targets) != 2 {
		t.Fatal(u)
	}
}
