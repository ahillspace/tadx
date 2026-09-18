// Package update implements coordinated CLI and bundled Guidance updates.
package update

import (
	"context"
	"fmt"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct {
	Check   bool
	Targets []string
}
type Release struct {
	Version string
	URL     string
}
type Output struct {
	InstallationPath string `json:"installation_path,omitempty"`
	Status           string `json:"status"`
	Version          string `json:"version"`
	LatestVersion    string `json:"latest_version,omitempty"`
	ReleaseURL       string `json:"release_url,omitempty"`
	Guidance         string `json:"guidance,omitempty"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }

type Runtime interface {
	Current() string
	ValidateTargets([]string) error
	Latest(context.Context) (Release, error)
	Install(context.Context, Release, []string) error
}
type Action struct{ runtime Runtime }

func New(runtime Runtime) *Action { return &Action{runtime: runtime} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	out := Output{Status: "incomplete"}
	fail := func(phase string, cause error) (Output, error) {
		advice := "Resolve the reported prerequisite; installation was not attempted."
		failurePhase, outcome := errs.PhaseSetup, errs.OutcomeNotAttempted
		if phase == "install" {
			advice = "Inspect the returned installation path and version before retrying. Installation may be partial; completed Guidance targets may contain the new bundle."
			failurePhase, outcome = errs.PhasePersistence, errs.OutcomeUnknown
		} else {
			out.Status = "not_attempted"
		}
		return out, &errs.Error{ID: "update." + phase + ".failed", Kind: errs.KindOperation, Operation: "update", Resource: out.InstallationPath, Summary: "TADX update did not complete.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: advice, Phase: failurePhase, Outcome: outcome}
	}
	if a == nil || a.runtime == nil {
		return fail("runtime", fmt.Errorf("update runtime is unavailable"))
	}
	out.Version = a.runtime.Current()
	if len(in.Targets) > 32 {
		return fail("validate", fmt.Errorf("at most 32 targets are supported"))
	}
	if err := a.runtime.ValidateTargets(in.Targets); err != nil {
		return fail("validate", err)
	}
	if target, ok := a.runtime.(interface{ InstallationTarget() (string, error) }); ok {
		path, err := target.InstallationTarget()
		out.InstallationPath = path
		if err != nil {
			return fail("target", err)
		}
	}
	release, err := a.runtime.Latest(ctx)
	if err != nil {
		return fail("check", err)
	}
	out.Status, out.LatestVersion, out.ReleaseURL = "checked", release.Version, release.URL
	if in.Check {
		return out, nil
	}
	if err = a.runtime.Install(ctx, release, in.Targets); err != nil {
		out.Status = "incomplete"
		return fail("install", err)
	}
	out.Status = "updated"
	out.Version = release.Version
	out.Guidance = "refreshed"
	return out, nil
}
