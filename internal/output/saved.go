package output

import (
	"reflect"

	"github.com/ahillspace/tadx/internal/commandhint"
)

// savedDetailHint changes the existing compact expansion marker, not action
// recovery instructions or resource fields. Saved output remains independent.
func savedDetailHint(value any, jsonOutput bool, configPath string) any {
	current := reflect.ValueOf(value)
	if !current.IsValid() {
		return value
	}
	args := []string{"last", "--full"}
	if jsonOutput {
		args = append(args, "--json")
	}
	command := commandhint.BindConfig(commandhint.Command(args...), configPath)
	cloned := bindHintReflect(current, "", command, make(map[visit]bool), 0)
	return cloned.Interface()
}
