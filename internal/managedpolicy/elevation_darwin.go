package managedpolicy

// Darwin's O_EXEC is not exposed by x/sys/unix. It opens an executable for
// inspection without requiring read permission on its contents.
const unixExecutableInspectFlags = 0x40000000
