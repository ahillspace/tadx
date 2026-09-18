package operationrun

// Launch starts a detached worker and returns without waiting for it.
//
// executable and args are passed directly to the operating system. No shell is
// involved. The launcher's current environment is inherited by the child only
// in memory and is never written by this package.
func Launch(executable string, args []string, workingDirectory string) error {
	return launch(executable, args, workingDirectory)
}
