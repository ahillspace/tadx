// Package capability registers the capability command domain.
package capability

import (
	"context"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Lister executes capability list.
type Lister interface {
	Execute(context.Context, capabilitylist.Input) (capabilitylist.Output, error)
}

// Getter executes capability get.
type Getter interface {
	Execute(context.Context, capabilityget.Input) (capabilityget.Output, error)
}

// Renderer writes structured action output.
type Renderer interface {
	Render(any) error
}

// Dependencies contains this domain's narrow wiring.
type Dependencies struct {
	Lister                Lister
	Getter                Getter
	Renderer              Renderer
	MutationsEnabled      bool
	ResolveMutationPolicy func(string) (bool, string, error)
	ListUse               string
	ListShort             string
	GetUse                string
	GetShort              string
}

// New creates the capability command and registers its implemented operations.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "capability",
		Short: "Discover TADX capabilities",
	}
	command.AddCommand(newList(deps), newGet(deps))
	command.PersistentFlags().String("environment", "", "configured environment selecting the site for mutation availability")
	return command
}

func newList(deps Dependencies) *cobra.Command {
	var domain string
	var resource string
	var owner string
	var product string
	var mutation bool
	var all bool
	var cursor string
	var limit int
	command := &cobra.Command{
		Use:   deps.ListUse,
		Short: deps.ListShort,
		Annotations: map[string]string{
			cliCapabilityAnnotation: "capability.list",
		},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("capability.list", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			var mutationFilter *bool
			if command.Flags().Changed("mutation") {
				mutationFilter = &mutation
			}
			full, jsonOutput := presentation(command)
			alias, _ := command.Flags().GetString("environment")
			input := capabilitylist.Input{
				Environment: alias,
				Domain:      domain, Resource: resource, Owner: owner, Product: product,
				Mutation: mutationFilter, All: all, Cursor: cursor, Limit: limit,
				Full: full, JSON: jsonOutput,
			}
			enabled := deps.MutationsEnabled
			if deps.ResolveMutationPolicy != nil {
				var err error
				enabled, _, err = deps.ResolveMutationPolicy(alias)
				if err != nil {
					// Capability metadata is local and remains useful even when
					// the effective policy cannot be established. Keep the
					// policy unavailable rather than treating false as disabled.
					input.MutationsEnabled = false
					partial, listErr := deps.Lister.Execute(command.Context(), input)
					if listErr != nil {
						return err
					}
					partial.MutationPolicy = "unavailable"
					return clierr.WithOutput(partial, err)
				}
			}
			input.MutationsEnabled = enabled
			output, err := deps.Lister.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(output)
		},
	}
	command.Flags().StringVar(&domain, "domain", "", "filter by exact command domain")
	command.Flags().StringVar(&resource, "resource", "", "filter by exact resource")
	command.Flags().StringVar(&owner, "owner", "", "filter by exact owner")
	command.Flags().StringVar(&product, "product", "", "filter by product availability")
	command.Flags().BoolVar(&mutation, "mutation", false, "filter by remote mutation status")
	command.Flags().BoolVar(&all, "all", false, "return all matching capabilities, up to 10000; cannot combine with --limit or --cursor")
	command.Flags().StringVar(&cursor, "cursor", "", "continue from a prior result cursor")
	command.Flags().IntVar(&limit, "limit", 0, "maximum capabilities to return, from 1 to 10000 (default 20)")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	return command
}

func presentation(command *cobra.Command) (full, jsonOutput bool) {
	full, _ = command.Flags().GetBool("full")
	jsonOutput, _ = command.Flags().GetBool("json")
	return full, jsonOutput
}

func newGet(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   deps.GetUse,
		Short: deps.GetShort,
		Annotations: map[string]string{
			cliCapabilityAnnotation: "capability.get",
		},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(command, args); err != nil {
				return clierr.Usage("capability.get", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			enabled := deps.MutationsEnabled
			if deps.ResolveMutationPolicy != nil {
				var err error
				alias, _ := command.Flags().GetString("environment")
				enabled, _, err = deps.ResolveMutationPolicy(alias)
				if err != nil {
					return err
				}
			}
			output, err := deps.Getter.Execute(command.Context(), capabilityget.Input{ID: args[0], MutationsEnabled: enabled})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(output)
		},
	}
}

const cliCapabilityAnnotation = "tadx.capability"
