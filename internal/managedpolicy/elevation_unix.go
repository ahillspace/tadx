//go:build linux || darwin

package managedpolicy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/capability"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

const installHelperArgument = "__tadx-policy-install"
const maxInstallReceipt = 64 << 10

type unixInstallReceipt struct {
	Result InstallResult `json:"result"`
	Error  string        `json:"error,omitempty"`
}

// The helper accepts only a directory and a built-in template. It does not load
// user configuration, accept policy payloads, or dispatch arbitrary commands.
func RunInstallHelper(args []string, definitions []capability.Definition) (bool, int) {
	if len(args) == 0 || args[0] != installHelperArgument {
		return false, 0
	}
	if len(args) != 3 || os.Geteuid() != 0 {
		return true, 1
	}
	out, err := runUnixInstall(context.Background(), InstallOptions{Directory: args[1], Template: args[2]}, definitions, true, nil, unixLocatorPath(), defaultUnixPolicyPath())
	receipt := unixInstallReceipt{Result: out}
	if err != nil {
		receipt.Error = err.Error()
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(receipt); encodeErr != nil {
		return true, 1
	}
	if err != nil {
		return true, 1
	}
	return true, 0
}

func elevateUnixInstall(ctx context.Context, out InstallResult) (InstallResult, error) {
	out.Phase = "elevation"
	if err := ctx.Err(); err != nil {
		return out, err
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return out, errors.New("administrator installation requires a terminal for sudo; run sudo tadx policy install with the same options from a terminal, or run as root")
	}
	defer tty.Close()
	if !term.IsTerminal(int(tty.Fd())) {
		return out, errors.New("administrator installation requires a terminal for sudo; run sudo tadx policy install with the same options from a terminal, or run as root")
	}
	// Never resolve sudo through a caller-controlled PATH.
	dir, _, err := openUnixDirectory("/usr/bin", false)
	if err != nil {
		return out, fmt.Errorf("cannot verify trusted /usr/bin/sudo: %w", err)
	}
	opened, err := inspectUnixExecutableAt(dir, "sudo")
	unix.Close(dir)
	if err != nil {
		if !opened {
			return out, fmt.Errorf("/usr/bin/sudo is unavailable; run tadx policy install as root: %w", err)
		}
		return out, fmt.Errorf("/usr/bin/sudo is not protected: %w", err)
	}
	executable, err := os.Executable()
	if err != nil {
		return out, err
	}
	cmd := exec.Command("/usr/bin/sudo", "--", executable, installHelperArgument, filepath.Dir(out.Path), out.Template)
	// sudo reads credentials directly from the controlling terminal. TADX never
	// receives them, and sudo diagnostics cannot contaminate the JSON receipt.
	cmd.Stdin, cmd.Stderr = tty, tty
	var receipt boundedReceipt
	cmd.Stdout = &receipt
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := cmd.Start(); err != nil {
		return out, fmt.Errorf("could not launch sudo installer: %w", err)
	}
	// Once launched, wait for the committing helper even if the caller cancels.
	waitErr := cmd.Wait()
	return decodeUnixInstallReceipt(out, receipt.Bytes(), receipt.overflow, waitErr)
}

func inspectUnixExecutableAt(dir int, name string) (bool, error) {
	fd, err := unix.Openat(dir, name, unixExecutableInspectFlags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return false, err
	}
	defer unix.Close(fd)
	return true, inspectUnixFD(fd, true)
}

type boundedReceipt struct {
	bytes.Buffer
	overflow bool
}

func (b *boundedReceipt) Write(data []byte) (int, error) {
	size := len(data)
	remaining := maxInstallReceipt - b.Len()
	if len(data) > remaining {
		b.overflow = true
		data = data[:remaining]
	}
	_, _ = b.Buffer.Write(data)
	return size, nil
}

func decodeUnixInstallReceipt(expected InstallResult, data []byte, overflow bool, waitErr error) (InstallResult, error) {
	var receipt unixInstallReceipt
	err := json.Unmarshal(data, &receipt)
	validPhase := false
	for _, phase := range []string{"validation", "prepare", "write", "verify", "locator", "complete"} {
		validPhase = validPhase || receipt.Result.Phase == phase
	}
	if overflow || err != nil || !validPhase || receipt.Result.Path != expected.Path || receipt.Result.Template != expected.Template {
		expected.Phase = "unknown"
		return expected, errors.New("sudo installation ended without a valid receipt (approval may have been denied); policy and locator changes are unknown; inspect the destination and run tadx policy status before retrying")
	}
	if receipt.Error != "" {
		return receipt.Result, fmt.Errorf("administrator policy installation failed during %s: %s", receipt.Result.Phase, receipt.Error)
	}
	if waitErr != nil || receipt.Result.Phase != "complete" || !receipt.Result.PolicyWritten || !receipt.Result.LocatorPublished || !receipt.Result.Active {
		receipt.Result.Phase = "unknown"
		return receipt.Result, errors.New("sudo installer did not confirm completion; inspect the destination and run tadx policy status before retrying")
	}
	return receipt.Result, nil
}
