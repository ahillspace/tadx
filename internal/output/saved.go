package output

import (
	"reflect"
	"slices"

	"github.com/ahillspace/tadx/internal/commandhint"
)

// detailHint changes the existing compact expansion marker, not action
// recovery instructions or resource fields. Saved output remains independent.
func detailHint(value any, args []string, jsonOutput bool, configPath string) any {
	current := reflect.ValueOf(value)
	if !current.IsValid() {
		return value
	}
	if jsonOutput {
		args = append(slices.Clone(args), "--json")
	}
	command := commandhint.BindConfig(commandhint.Command(args...), configPath)
	cloned := bindHintReflect(current, "", command, make(map[visit]bool), 0)
	return cloned.Interface()
}
