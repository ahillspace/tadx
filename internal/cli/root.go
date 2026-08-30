// Package cli contains thin Cobra plumbing for TADX.
package cli

import (
	"context"
	"fmt"
	"strings"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

// CapabilityAnnotation associates an executable command with its registry ID.
const CapabilityAnnotation = "tadx.capability"

// RegisteredCommand describes a capability discovered from the actual Cobra tree.
type RegisteredCommand struct {
	CapabilityID string
	CommandPath  []string
}

// Lister executes capability list.
type Lister interface {
	Execute(context.Context, capabilitylist.Input) (capabilitylist.Output, error)
}

// Getter executes capability get.
type Getter interface {
	Execute(context.Context, capabilityget.Input) (capabilityget.Output, error)
}

// Renderer writes structured command output.
type Renderer interface {
	Render(any) error
}

// Dependencies contains the explicitly wired Phase 0 command dependencies.
type Dependencies struct {
	Lister           Lister
	Getter           Getter
	Renderer         Renderer
	MutationsEnabled bool
	ListUse          string
	ListShort        string
	GetUse           string
	GetShort         string
}

// NewRoot creates the root command. It contains no domain behavior.
func NewRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "tadx",
		Short:         "Deterministic Tableau lifecycle and development CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(capabilitycli.New(capabilitycli.Dependencies{
		Lister:           deps.Lister,
		Getter:           deps.Getter,
		Renderer:         deps.Renderer,
		MutationsEnabled: deps.MutationsEnabled,
		ListUse:          deps.ListUse,
		ListShort:        deps.ListShort,
		GetUse:           deps.GetUse,
		GetShort:         deps.GetShort,
	}))
	root.CompletionOptions.DisableDefaultCmd = true
	setFlagErrorHandlers(root)
	return root
}

// RegisteredCommands discovers capability bindings from the actual command tree.
func RegisteredCommands(root *cobra.Command) ([]RegisteredCommand, error) {
	var registrations []RegisteredCommand
	var walk func(*cobra.Command) error
	walk = func(command *cobra.Command) error {
		id := command.Annotations[CapabilityAnnotation]
		runnable := command.Run != nil || command.RunE != nil
		if runnable && id == "" {
			return fmt.Errorf("runnable command %q has no capability annotation", command.CommandPath())
		}
		if !runnable && id != "" {
			return fmt.Errorf("non-runnable command %q has capability annotation %q", command.CommandPath(), id)
		}
		if id != "" {
			path := strings.Fields(command.CommandPath())
			if len(path) > 0 {
				path = path[1:]
			}
			registrations = append(registrations, RegisteredCommand{CapabilityID: id, CommandPath: path})
		}
		for _, child := range command.Commands() {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return registrations, nil
}

func setFlagErrorHandlers(command *cobra.Command) {
	command.SetFlagErrorFunc(func(command *cobra.Command, cause error) error {
		return &errs.Error{Kind: errs.KindUsage, Operation: command.CommandPath(), Summary: cause.Error(), Cause: cause}
	})
	for _, child := range command.Commands() {
		setFlagErrorHandlers(child)
	}
}
