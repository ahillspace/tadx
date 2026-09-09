package create

import "github.com/ahillspace/tadx/internal/commandhint"

func permissionHint(in Input) string {
	args := []string{"admin", "permission", "inspect", "--kind", in.ResourceKind, "--id", in.ResourceLUID, "--full"}
	for _, flag := range []struct{ name, value string }{{"--default-for", in.DefaultFor}, {"--principal-type", in.PrincipalType}, {"--principal-id", in.PrincipalLUID}, {"--capability", in.Capability}} {
		if flag.value != "" {
			args = append(args, flag.name, flag.value)
		}
	}
	return commandhint.Environment(in.Environment, args...)
}
