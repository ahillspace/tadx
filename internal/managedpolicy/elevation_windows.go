package managedpolicy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"github.com/ahillspace/tadx/internal/capability"
	"golang.org/x/sys/windows"
)

const installHelperArgument = "__tadx-policy-install"

// RunInstallHelper dispatches only the bounded installer before normal CLI setup.
// The helper cannot run arbitrary commands, read user configuration, or elevate itself.
func RunInstallHelper(args []string, definitions []capability.Definition) (bool, int) {
	if len(args) == 0 || args[0] != installHelperArgument {
		return false, 0
	}
	if len(args) != 3 || !windows.GetCurrentProcessToken().IsElevated() {
		return true, 1
	}
	out, data, err := prepareInstall(InstallOptions{Directory: args[1], Template: args[2]}, definitions)
	if err == nil {
		out, err = installWith(context.Background(), out, data, &windowsInstaller{root: windowsRegistryRoot(), key: locatorKey, definitions: definitions})
	}
	return true, encodeInstallExit(out, err)
}

var installPhases = []string{"validation", "prepare", "write", "verify", "locator"}

// Native error numbers and stable domain reasons cross the process boundary,
// without trusting a caller-writable receipt file or parsing console output.
var installFailureReasons = []string{
	"choose a dedicated policy directory, not a shared system directory or user home",
	"the drive root is not a dedicated policy directory",
	"a user home or the profiles directory is not a dedicated policy directory",
	"policy directory must be a clean local absolute path",
	"policy directory cannot use trailing spaces or dots",
	"policy installation requires a dedicated directory containing no unrelated files or directories",
	"policy installation path must contain only ordinary directories, without reparse points",
	"existing policy must be an ordinary file with one link",
	"policy locator owner must be SYSTEM, Administrators, or TrustedInstaller",
	"policy locator permits non-administrator changes",
	"policy locator requires a non-null DACL",
	"unsupported policy locator DACL entry",
	"cannot inspect policy locator protection",
	"owner must be SYSTEM, Administrators, or TrustedInstaller",
	"DACL grants modification rights to a non-administrator principal",
	"a non-null DACL is required",
	"unsupported effective DACL entry",
	"reparse points are not permitted",
	"policy must be a regular file with one link",
	"unknown policy template",
	"cannot read owner and DACL",
	"cannot inspect managed policy path",
	"cannot open managed policy path for security inspection",
	"cannot inspect DACL entries",
	"cannot read managed policy",
	"managed policy exceeds 1 MiB",
	"the protected policy locator requires a Directory REG_SZ value",
	"policy locator verification returned a different directory",
	"cannot acquire policy installation lock",
}

func installErrorCode(err error) uint32 {
	if native, ok := errors.AsType[syscall.Errno](err); ok && uint64(native) <= 65535 {
		return 1<<16 | uint32(native)
	}
	for index, reason := range installFailureReasons {
		if strings.Contains(err.Error(), reason) {
			return 2<<16 | uint32(index)
		}
	}
	return 0
}

func installErrorReason(code uint32) string {
	switch (code >> 16) & 15 {
	case 1:
		return syscall.Errno(code & 65535).Error()
	case 2:
		if index := int(code & 65535); index < len(installFailureReasons) {
			return installFailureReasons[index]
		}
	}
	return "an unexpected installer error occurred"
}

func encodeInstallExit(out InstallResult, err error) int {
	if err == nil {
		return 0
	}
	for index, phase := range installPhases {
		if out.Phase != phase {
			continue
		}
		flags := 0
		if out.PolicyWritten {
			flags |= 1
		}
		if out.LocatorPublished {
			flags |= 2
		}
		if out.Active {
			flags |= 4
		}
		if out.ProtectionChanged {
			flags |= 8
		}
		return int(0xA0000000 | uint32(index)<<24 | uint32(flags)<<20 | installErrorCode(err))
	}
	return 1
}

func decodeInstallExit(out InstallResult, code uint32) (InstallResult, error) {
	if code == 0 {
		out.Phase = "complete"
		out.PolicyWritten = true
		out.LocatorPublished = true
		out.Active = true
		out.ProtectionChanged = true
		return out, nil
	}
	if code>>28 == 0xA && (code>>24)&15 < uint32(len(installPhases)) {
		out.Phase = installPhases[(code>>24)&15]
		flags := (code >> 20) & 15
		out.PolicyWritten = flags&1 != 0
		out.LocatorPublished = flags&2 != 0
		out.Active = flags&4 != 0
		out.ProtectionChanged = flags&8 != 0
		return out, fmt.Errorf("elevated policy installation failed during %s: %s; inspect the destination and run tadx policy status before retrying", out.Phase, installErrorReason(code))
	}
	out.Phase = "unknown"
	return out, errors.New("the elevated installer ended without a receipt; policy and locator changes are unknown; inspect the destination and run tadx policy status")
}

type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Window     windows.Handle
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	Instance   windows.Handle
	IDList     unsafe.Pointer
	Class      *uint16
	ClassKey   windows.Handle
	HotKey     uint32
	Icon       windows.Handle
	Process    windows.Handle
}

func elevateInstall(ctx context.Context, out InstallResult) (InstallResult, error) {
	out.Phase = "elevation"
	if err := ctx.Err(); err != nil {
		return out, err
	}
	executable, err := os.Executable()
	if err != nil {
		return out, err
	}
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return out, err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{installHelperArgument, filepath.Dir(out.Path), out.Template}))
	if err != nil {
		return out, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil {
		return out, err
	}
	defer windows.CoUninitialize()
	info := shellExecuteInfo{Size: uint32(unsafe.Sizeof(shellExecuteInfo{})), Mask: 0x40 | 0x100 | 0x400, Verb: windows.StringToUTF16Ptr("runas"), File: file, Parameters: parameters, Show: windows.SW_HIDE}
	result, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(file)
	runtime.KeepAlive(parameters)
	if result == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return out, errors.New("administrator approval was cancelled; the installer did not start")
		}
		return out, fmt.Errorf("could not launch the administrator installer: %w", callErr)
	}
	if info.Process == 0 {
		out.Phase = "unknown"
		return out, errors.New("the elevated installer returned no process handle; installation outcome is unknown")
	}
	defer windows.CloseHandle(info.Process)
	// After launch, cancellation must not fabricate rollback or kill a committing helper.
	if _, err := windows.WaitForSingleObject(info.Process, windows.INFINITE); err != nil {
		out.Phase = "unknown"
		return out, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.Process, &code); err != nil {
		out.Phase = "unknown"
		return out, err
	}
	return decodeInstallExit(out, code)
}
