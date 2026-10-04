package auth

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "auth", Note: "Authentication uses PATs. Login prompts for credentials and stores them in the OS credential store.", Common: true, Examples: []string{}},
	}
}
