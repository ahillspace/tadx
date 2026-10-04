// Package helpmeta attaches presentation-only facts to registered Cobra commands.
package helpmeta

import (
	"strings"

	"github.com/spf13/cobra"
)

// FlagAnnotation copies presentation values without changing accepted syntax.
func FlagAnnotation(command *cobra.Command, name, key string, values []string) {
	flag := command.Flags().Lookup(name)
	if flag == nil {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[key] = append([]string(nil), values...)
}

// Choices records the finite values displayed by help.
func Choices(command *cobra.Command, name string, values ...string) {
	FlagAnnotation(command, name, "tadx.help.choices", values)
}

// Value describes a flag value's presentation syntax.
func Value(command *cobra.Command, name, format string) {
	FlagAnnotation(command, name, "tadx.help.value", []string{format})
}

// FlagNote appends an idempotent usage note.
func FlagNote(command *cobra.Command, name, text string) {
	flag := command.Flags().Lookup(name)
	if flag != nil && !strings.Contains(flag.Usage, text) {
		flag.Usage += "; " + text
	}
}

// Required marks existing flags as required in help, without changing parsing.
func Required(command *cobra.Command, names ...string) {
	for _, name := range names {
		FlagAnnotation(command, name, "tadx.help.required", []string{"true"})
	}
}

// Group records a relation between the flags present on this command.
func Group(command *cobra.Command, relation string, names ...string) {
	var present []string
	for _, name := range names {
		if command.Flags().Lookup(name) != nil {
			present = append(present, name)
		}
	}
	if len(present) < 2 {
		return
	}
	for _, name := range present {
		FlagAnnotation(command, name, "tadx.help."+relation, []string{strings.Join(present, " ")})
	}
}

// Constraint records an idempotent command-level explanation.
func Constraint(command *cobra.Command, text string) {
	if command.Annotations == nil {
		command.Annotations = map[string]string{}
	}
	if !strings.Contains(command.Annotations["tadx.help.constraints"], text) {
		command.Annotations["tadx.help.constraints"] = strings.TrimSpace(command.Annotations["tadx.help.constraints"] + "\n" + text)
	}
}

// Summary records the compact description used by category navigation.
func Summary(command *cobra.Command, text string) {
	if command.Annotations == nil {
		command.Annotations = map[string]string{}
	}
	command.Annotations["tadx.help.summary"] = text
}
