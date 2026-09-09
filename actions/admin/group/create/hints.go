package create

import "github.com/ahillspace/tadx/internal/commandhint"

func recoveryHint(input Input, id string) string {
	if id != "" {
		return commandhint.Environment(input.Environment, "admin", "group", "inspect", "--id", id)
	}
	return commandhint.Environment(input.Environment, "admin", "group", "inspect", "--name", input.Name)
}
