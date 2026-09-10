package commandhint

import "strings"

// BindConfig attaches non-secret configuration context to a generated suggestion.
// Only the command prefix changes; already-quoted arguments are never re-parsed
// or reconstructed. Callers must restrict this to help and recovery fields.
func BindConfig(text, configPath string) string {
	if configPath == "" {
		return text
	}
	start := strings.Index(text, "tadx ")
	if start < 0 || (start > 0 && !strings.ContainsRune(" \t\n:`,(", rune(text[start-1]))) {
		return text
	}
	args := text[start+len("tadx "):]
	if hasConfigFlag(args) {
		return text
	}
	return text[:start] + Command("--config", configPath) + " " + args
}

// hasConfigFlag recognizes only unquoted flag tokens, not text inside a resource
// name. It does not execute shell syntax or recover arguments from shell code.
func hasConfigFlag(args string) bool {
	var quote byte
	for i := 0; i < len(args); i++ {
		ch := args[i]
		if quote != 0 {
			if ch == quote {
				if i+1 < len(args) && args[i+1] == quote {
					i++
				} else {
					quote = 0
				}
			} else if ch == '\\' && quote == '"' && i+1 < len(args) {
				i++
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			if (i == 0 || args[i-1] == ' ' || args[i-1] == '\t') && strings.HasPrefix(args[i+1:], "--config") {
				end := i + 1 + len("--config")
				if end < len(args) && (args[end] == '=' || args[end] == ch) {
					return true
				}
			}
			quote = ch
			continue
		}
		if ch == ';' || ch == '\n' || ch == '|' {
			break
		}
		if (i == 0 || args[i-1] == ' ' || args[i-1] == '\t') && strings.HasPrefix(args[i:], "--config") {
			end := i + len("--config")
			if end == len(args) || strings.ContainsRune("= \t", rune(args[end])) {
				return true
			}
		}
	}
	return false
}
