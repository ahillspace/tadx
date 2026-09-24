package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// ValidateArguments rejects ambiguous separate flag values before Cobra can
// consume a following option as data. Explicit --flag=value remains available
// when a value intentionally starts with a dash.
func ValidateArguments(command *cobra.Command, args []string) error {
	if command == nil {
		return nil
	}
	for index := 0; index < len(args); index++ {
		token := args[index]
		if token == "--" {
			break
		}
		flag, separate := argumentFlag(command, token)
		if flag == nil || !separate || flag.NoOptDefVal != "" {
			continue
		}
		if index+1 == len(args) {
			continue // Cobra reports the missing value.
		}
		value := args[index+1]
		if len(value) > 1 && strings.HasPrefix(value, "-") && !validNegativeNumber(flag, value) {
			cause := fmt.Errorf("--%s requires a value; use --%s=<value> for an intentional value beginning with '-'", flag.Name, flag.Name)
			return withUsageRecovery(command, clierr.Usage(command.CommandPath(), cause))
		}
		index++
	}
	return nil
}

func argumentFlag(command *cobra.Command, token string) (*pflag.Flag, bool) {
	if strings.HasPrefix(token, "--") {
		name, _, attached := strings.Cut(token[2:], "=")
		if name == "" {
			return nil, false
		}
		if flag := command.Flags().Lookup(name); flag != nil {
			return flag, !attached
		}
		return command.InheritedFlags().Lookup(name), !attached
	}
	if len(token) > 1 && token[0] == '-' {
		for index := 1; index < len(token); index++ {
			name := token[index : index+1]
			flag := command.Flags().ShorthandLookup(name)
			if flag == nil {
				flag = command.InheritedFlags().ShorthandLookup(name)
			}
			if flag == nil {
				return nil, false // Cobra reports unknown shorthand flags.
			}
			if flag.NoOptDefVal == "" {
				return flag, index == len(token)-1
			}
		}
	}
	return nil, false
}

func validNegativeNumber(flag *pflag.Flag, value string) bool {
	if len(value) < 2 || value[0] != '-' {
		return false
	}
	switch flag.Value.Type() {
	case "int", "int8", "int16", "int32", "int64", "float32", "float64":
		_, err := strconv.ParseFloat(value, 64)
		return err == nil
	default:
		return false
	}
}
