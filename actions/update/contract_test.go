package update

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

type recordingRuntime struct {
	calls   []string
	failure string
	cause   error
	targets []string
}

func (r *recordingRuntime) Current() string {
	r.calls = append(r.calls, "current")
	return "1.0.0"
}

func (r *recordingRuntime) observe(call string) error {
	r.calls = append(r.calls, call)
	if r.failure == call {
		return r.cause
	}
	return nil
}

func (r *recordingRuntime) ValidateTargets(targets []string) error {
	r.targets = targets
	return r.observe("validate")
}

func (r *recordingRuntime) InstallationTarget() (string, error) {
	return "/fixture/candidate-build", r.observe("target")
}

func (r *recordingRuntime) Latest(context.Context) (Release, error) {
	return Release{Version: "1.1.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.1.0"}, r.observe("check")
}

func (r *recordingRuntime) Install(_ context.Context, _ Release, targets []string) error {
	r.targets = targets
	return r.observe("install")
}

func TestUpdateOrderingAndFailureContext(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   Input
		failure string
		calls   []string
		status  string
		path    string
		latest  string
	}{
		{name: "check arbitrary basename", input: Input{Check: true}, calls: []string{"current", "validate", "target", "check"}, status: "checked", path: "/fixture/candidate-build", latest: "1.1.0"},
		{name: "too many targets", input: Input{Targets: make([]string, 33)}, failure: "validate", calls: []string{"current"}, status: "not_attempted"},
		{name: "invalid target", failure: "validate", calls: []string{"current", "validate"}, status: "not_attempted"},
		{name: "check target failure", input: Input{Check: true}, failure: "target", calls: []string{"current", "validate", "target"}, status: "not_attempted", path: "/fixture/candidate-build"},
		{name: "release failure", failure: "check", calls: []string{"current", "validate", "target", "check"}, status: "not_attempted", path: "/fixture/candidate-build"},
		{name: "installation failure", failure: "install", calls: []string{"current", "validate", "target", "check", "install"}, status: "incomplete", path: "/fixture/candidate-build", latest: "1.1.0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &recordingRuntime{failure: test.failure, cause: errors.New("fixture failure")}
			out, err := New(runtime).Execute(t.Context(), test.input)
			if !slices.Equal(runtime.calls, test.calls) {
				t.Fatalf("calls = %v, want %v", runtime.calls, test.calls)
			}
			if out.Status != test.status || out.Version != "1.0.0" || out.InstallationPath != test.path || out.LatestVersion != test.latest || out.Guidance != "" {
				t.Fatalf("output = %+v", out)
			}
			if (out.ReleaseURL != "") != (test.latest != "") {
				t.Fatalf("release URL = %q", out.ReleaseURL)
			}
			if test.failure == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.ID != "update."+test.failure+".failed" || failure.Resource != test.path || failure.Retryable == nil || *failure.Retryable {
				t.Fatalf("error = %+v", err)
			}
			phase, outcome := errs.PhaseSetup, errs.OutcomeNotAttempted
			if test.failure == "install" {
				phase, outcome = errs.PhasePersistence, errs.OutcomeUnknown
			}
			if failure.Phase != phase || failure.Outcome != outcome {
				t.Fatalf("failure = %+v", failure)
			}
			if len(test.input.Targets) <= 32 && !errors.Is(err, runtime.cause) {
				t.Fatalf("cause lost: %v", err)
			}
		})
	}
}

func TestUpdatePreservesTargetOrderAndDuplicates(t *testing.T) {
	targets := []string{"claude", "codex", "claude"}
	runtime := &recordingRuntime{}
	out, err := New(runtime).Execute(t.Context(), Input{Targets: targets})
	if err != nil || out.Status != "updated" || !slices.Equal(runtime.targets, targets) {
		t.Fatalf("output=%+v error=%v targets=%v", out, err, runtime.targets)
	}
}
