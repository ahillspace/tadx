package commandhint

import "strings"

// Recovery formats structured repair suggestions without parsing quoted command text.
func Recovery(commands [][]string, explanation, configPath string) string {
	parts := make([]string, 0, len(commands))
	for _, args := range commands {
		if configPath != "" {
			args = append([]string{"--config", configPath}, args...)
		}
		parts = append(parts, Command(args...))
	}
	advice := "Repair commands: " + strings.Join(parts, "; ") + "."
	if explanation != "" {
		advice += " " + explanation
	}
	return advice
}
