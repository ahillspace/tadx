// Package version owns the installed-version and release-check operation.
package version

import (
	"context"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/errs"
	versioncore "github.com/ahillspace/tadx/internal/version"
)

type Input struct{ Check bool }
type Output struct {
	Status          string   `json:"status"`
	Version         string   `json:"version"`
	LatestVersion   string   `json:"latest_version,omitempty"`
	UpdateAvailable bool     `json:"update_available,omitempty"`
	ReleaseURL      string   `json:"release_url,omitempty"`
	PublishedAt     string   `json:"published_at,omitempty"`
	Help            []string `json:"help,omitempty"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }

type ReleaseChecker interface {
	Latest(context.Context) (versioncore.Release, error)
}
type Service struct {
	current string
	checker ReleaseChecker
}

func New(current string, checker ReleaseChecker) *Service {
	return &Service{current: current, checker: checker}
}
func (a *Service) Get(ctx context.Context, in Input) (Output, error) {
	current := a.current
	out := Output{Status: "installed", Version: current, Help: []string{"tadx version --check"}}
	if !in.Check {
		return out, nil
	}
	out.Status, out.Help = "check_unavailable", nil
	release, err := a.checker.Latest(ctx)
	if err != nil {
		return out, &errs.Error{ID: "version.get.check.failed", Kind: errs.KindOperation, Operation: "version.get", Summary: "Release check failed; installed version is retained in the result.", Cause: err, Retryable: errs.Bool(true), CorrectiveAction: "Retry the release check later if current release information is needed."}
	}
	out.LatestVersion = release.Version
	out.ReleaseURL = release.URL
	if !release.PublishedAt.IsZero() {
		out.PublishedAt = release.PublishedAt.UTC().Format(time.RFC3339)
	}
	out.UpdateAvailable = current != "dev" && compare(current, release.Version) < 0
	if out.UpdateAvailable {
		out.Status, out.Help = "update-available", []string{"tadx update"}
	} else {
		out.Status = "current"
	}
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
