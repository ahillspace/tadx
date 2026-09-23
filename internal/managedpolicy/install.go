package managedpolicy

import (
	"context"
	"path/filepath"
)

// InstallOptions select a fresh template and destination, independently of the active locator.
type InstallOptions struct {
	Directory string
	Template  string
}

// InstallResult records confirmed changes, including failures after replacement.
type InstallResult struct {
	Path              string `json:"path"`
	Template          string `json:"template"`
	PolicyWritten     bool   `json:"policy_written"`
	LocatorPublished  bool   `json:"locator_published"`
	Active            bool   `json:"active"`
	ProtectionChanged bool   `json:"protection_changed"`
	Phase             string `json:"phase"`
}

type installBackend interface {
	lock(context.Context) (func(), error)
	current() (string, error)
	prepare(string) (func(), bool, error)
	write(string, []byte) error
	verify(string) error
	publish(string) (bool, error)
}

func installWith(ctx context.Context, out InstallResult, data []byte, backend installBackend) (InstallResult, error) {
	out.Phase = "prepare"
	if err := ctx.Err(); err != nil {
		return out, err
	}
	unlock, err := backend.lock(ctx)
	if err != nil {
		return out, err
	}
	defer unlock()
	// A broken existing locator must not prevent administrator recovery.
	previous, _ := backend.current()
	closePaths, changed, err := backend.prepare(filepath.Dir(out.Path))
	out.ProtectionChanged = changed
	if closePaths != nil {
		defer closePaths()
	}
	if err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	out.Phase = "write"
	if err := backend.write(out.Path, data); err != nil {
		return out, err
	}
	out.PolicyWritten = true
	out.Active = sameInstallPath(previous, out.Path)
	out.Phase = "verify"
	if err := backend.verify(out.Path); err != nil {
		return out, err
	}
	out.Phase = "locator"
	published, err := backend.publish(filepath.Dir(out.Path))
	out.LocatorPublished = published
	if published {
		out.Active = true
	}
	if err != nil {
		return out, err
	}
	out.Phase = "complete"
	return out, nil
}
