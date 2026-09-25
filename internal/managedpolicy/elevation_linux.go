package managedpolicy

import "golang.org/x/sys/unix"

// O_PATH permits descriptor-based metadata inspection without file read access.
const unixExecutableInspectFlags = unix.O_PATH
