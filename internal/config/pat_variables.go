package config

// ConfiguredPATVariables lists explicit PAT variable references for child-process
// filtering. Load the selected path when starting a child, after flag parsing.
// An unreadable configuration returns no explicit names so an updater can still
// repair an installation; its conventional PAT-name filtering remains required.
func ConfiguredPATVariables(path string) []string {
	configuration, err := Load(path)
	if err != nil {
		return nil
	}
	names := make([]string, 0, 2*len(configuration.Environments))
	for _, environment := range configuration.Environments {
		names = append(names, environment.Auth.PATNameEnv, environment.Auth.PATSecretEnv)
	}
	return names
}
