// Package get implements version.get.
package get

import (
	"context"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct{ Check bool }
type Release struct {
	Version     string
	URL         string
	PublishedAt time.Time
}
type Output struct {
	Status          string   `json:"status"`
	Version         string   `json:"version"`
	LatestVersion   string   `json:"latest_version,omitempty"`
	UpdateAvailable bool     `json:"update_available,omitempty"`
	ReleaseURL      string   `json:"release_url,omitempty"`
	PublishedAt     string   `json:"published_at,omitempty"`
	Help            []string `json:"help"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }

type CurrentProvider interface{ Current() string }
type ReleaseChecker interface {
	Latest(context.Context) (Release, error)
}
type Action struct {
	current CurrentProvider
	checker ReleaseChecker
}

func New(current CurrentProvider, checker ReleaseChecker) *Action {
	return &Action{current: current, checker: checker}
}
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if a == nil || a.current == nil {
		return Output{}, &errs.Error{ID: "version.get.runtime", Kind: errs.KindRuntime, Operation: "version.get", Summary: "Version inspection is not configured.", Retryable: errs.Bool(false)}
	}
	current := strings.TrimSpace(a.current.Current())
	if current == "" {
		return Output{}, &errs.Error{ID: "version.get.runtime", Kind: errs.KindRuntime, Operation: "version.get", Summary: "Current version identity is empty.", Retryable: errs.Bool(false)}
	}
	out := Output{Status: "installed", Version: current, Help: []string{"tadx version --check"}}
	if !in.Check {
		return out, nil
	}
	if a.checker == nil {
		return Output{}, &errs.Error{ID: "version.get.check.runtime", Kind: errs.KindRuntime, Operation: "version.get", Summary: "Release checking is not configured.", Retryable: errs.Bool(false)}
	}
	release, err := a.checker.Latest(ctx)
	if err != nil {
		return Output{}, &errs.Error{ID: "version.get.check.failed", Kind: errs.KindOperation, Operation: "version.get", Summary: "Release check failed.", Cause: err, Retryable: errs.Bool(true), CorrectiveAction: "Retry later or run tadx version without --check for offline version information."}
	}
	out.LatestVersion = release.Version
	out.ReleaseURL = release.URL
	if !release.PublishedAt.IsZero() {
		out.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)
	}
	out.UpdateAvailable = current != "dev" && compare(current, release.Version) < 0
	if out.UpdateAvailable {
		out.Status = "update-available"
	} else {
		out.Status = "current"
	}
	out.Help = nil
	return out, nil
}
func compare(left, right string) int {
	parse := func(value string) [3]int {
		var out [3]int
		parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
		for i := 0; i < len(parts) && i < 3; i++ {
			for _, r := range parts[i] {
				if r < '0' || r > '9' {
					break
				}
				out[i] = out[i]*10 + int(r-'0')
			}
		}
		return out
	}
	a, b := parse(left), parse(right)
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
