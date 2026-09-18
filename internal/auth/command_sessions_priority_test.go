package auth_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/lock"
)

func TestCommandSessionForegroundPriorityAcrossProcesses(t *testing.T) {
	if os.Getenv("TADX_FOREGROUND_PRIORITY_CHILD") == "1" {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		manager := auth.NewCommandSessions(nil, nil, os.Getenv("TADX_FOREGROUND_PRIORITY_DIRECTORY"))
		defer manager.Close()
		target := commandTarget()
		target.SiteContentURL = "foreground-site"
		if _, err := manager.AuthenticateCredentials(ctx, target, commandCredential(), &commandSigner{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("TADX_FOREGROUND_PRIORITY_READY"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		_, _ = io.CopyN(io.Discard, os.Stdin, 1)
		return
	}
	for _, terminate := range []bool{false, true} {
		name := "normal_exit"
		if terminate {
			name = "terminated_waiter"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			holder := auth.NewCommandSessions(nil, nil, directory)
			defer holder.Close()
			if _, err := holder.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), &commandSigner{}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			ready := filepath.Join(t.TempDir(), "foreground-ready")
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandSessionForegroundPriorityAcrossProcesses$")
			child.Env = append(os.Environ(), "TADX_FOREGROUND_PRIORITY_CHILD=1", "TADX_FOREGROUND_PRIORITY_DIRECTORY="+directory, "TADX_FOREGROUND_PRIORITY_READY="+ready)
			var output bytes.Buffer
			child.Stdout, child.Stderr = &output, &output
			stdin, err := child.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				_ = stdin.Close()
				if !waited {
					_ = child.Process.Kill()
					_ = child.Wait()
				}
			}()
			waitForForegroundAdmission(t, directory)

			monitor := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
			defer monitor.Close()
			monitorCtx, cancelMonitor := context.WithCancel(ctx)
			defer cancelMonitor()
			monitorDone := make(chan error, 1)
			go func() {
				_, err := monitor.AuthenticateMonitor(monitorCtx, commandEnvironmentTarget(), &commandSigner{})
				monitorDone <- err
			}()
			if terminate {
				// A killed foreground process must not leave a stale registration
				// that prevents future monitoring from using this PAT.
				if err := child.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_ = child.Wait()
				waited = true
			}
			if err := holder.Suspend(ctx); err != nil {
				t.Fatal(err)
			}
			if !terminate {
				deadline := time.NewTimer(5 * time.Second)
				defer deadline.Stop()
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
			waitForForeground:
				for {
					if _, err := os.Stat(ready); err == nil {
						break waitForForeground
					}
					select {
					case err := <-monitorDone:
						t.Fatalf("monitor passed a registered foreground process: %v", err)
					case <-deadline.C:
						t.Fatal("foreground process did not authenticate")
					case <-ticker.C:
					}
				}
				select {
				case err := <-monitorDone:
					t.Fatalf("monitor acquired a credential still held by foreground: %v", err)
				default:
				}
				_ = stdin.Close()
				err := child.Wait()
				waited = true
				if err != nil {
					t.Fatalf("foreground process: %v\n%s", err, output.String())
				}
			}
			select {
			case err := <-monitorDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("monitor did not resume after foreground exit")
			}
		})
	}
}

func TestCommandSessionCanceledForegroundReleasesAdmission(t *testing.T) {
	directory := t.TempDir()
	holder := auth.NewCommandSessions(nil, nil, directory)
	defer holder.Close()
	if _, err := holder.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), &commandSigner{}); err != nil {
		t.Fatal(err)
	}
	foreground := auth.NewCommandSessions(nil, nil, directory)
	defer foreground.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := foreground.AuthenticateCredentials(ctx, commandTarget(), commandCredential(), &commandSigner{})
		done <- err
	}()
	waitForForegroundAdmission(t, directory)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled foreground error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("foreground cancellation did not complete")
	}
	if err := holder.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}
	monitor := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
	defer monitor.Close()
	monitorCtx, cancelMonitor := context.WithTimeout(t.Context(), time.Second)
	defer cancelMonitor()
	if _, err := monitor.AuthenticateMonitor(monitorCtx, commandEnvironmentTarget(), &commandSigner{}); err != nil {
		t.Fatalf("canceled foreground left a stale admission: %v", err)
	}
}

func waitForForegroundAdmission(t *testing.T, directory string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		paths, err := filepath.Glob(filepath.Join(directory, "*.admission.lock"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			held, err := lock.TryAcquire(path)
			if errors.Is(err, lock.ErrLocked) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := held.Release(); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("foreground command did not register shared admission")
}
