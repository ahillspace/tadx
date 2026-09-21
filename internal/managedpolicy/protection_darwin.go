package managedpolicy

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

func SystemPath() (string, error) {
	return "/Library/Application Support/TADX/managed-policy.json", nil
}

// fgetattrlist binds ACL inspection to the descriptor opened with O_NOFOLLOW.
// The native kauth_filesec layout and rights come from Apple's sys/kauth.h.
func checkExtendedACL(fd int) error {
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_EXTENDED_SECURITY}
	buffer := make([]byte, 16<<10)
	_, _, errno := unix.Syscall6(unix.SYS_FGETATTRLIST, uintptr(fd), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), unix.FSOPT_REPORT_FULLSIZE, 0)
	runtime.KeepAlive(&attrs)
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return errors.New("cannot inspect extended ACL")
	}
	return checkDarwinACLBuffer(buffer)
}
