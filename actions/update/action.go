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
	Status        string `json:"status"`
	Version       string `json:"version"`
	LatestVersion string `json:"latest_version,omitempty"`
	ReleaseURL    string `json:"release_url,omitempty"`
	Guidance      string `json:"guidance,omitempty"`
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
	fail := func(phase string, cause error) (Output, error) {
		return Output{}, &errs.Error{ID: "update." + phase + ".failed", Kind: errs.KindOperation, Operation: "update", Summary: "TADX update did not complete.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Check installation access and GitHub release availability, then retry tadx update. Completed Guidance targets may already contain the new bundle."}
	}
	if a == nil || a.runtime == nil {
		return fail("runtime", fmt.Errorf("update runtime is unavailable"))
	}
	if len(in.Targets) > 32 {
		return fail("validate", fmt.Errorf("at most 32 targets are supported"))
	}
	if err := a.runtime.ValidateTargets(in.Targets); err != nil {
		return fail("validate", err)
	}
	release, err := a.runtime.Latest(ctx)
	if err != nil {
		return fail("check", err)
	}
	out := Output{Status: "checked", Version: a.runtime.Current(), LatestVersion: release.Version, ReleaseURL: release.URL}
	if in.Check {
		return out, nil
	}
	if err = a.runtime.Install(ctx, release, in.Targets); err != nil {
		return fail("install", err)
	}
	out.Status = "updated"
	out.Version = release.Version
	out.Guidance = "refreshed"
	return out, nil
}
