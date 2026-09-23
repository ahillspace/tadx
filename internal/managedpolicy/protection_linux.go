package managedpolicy

func defaultUnixPolicyPath() string { return "/etc/tadx/managed-policy.json" }

// Linux POSIX ACL named-user/group effective permissions are bounded by the
// mask reflected in the file's group mode bits. Rejecting group write therefore
// rejects effective ACL writes, including named principals. Default directory
// ACLs cannot change already-created entries, which are checked individually.
func checkExtendedACL(_ int) error { return nil }
