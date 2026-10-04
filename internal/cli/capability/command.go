// Package capability registers the capability command domain.
package capability

import (
	"context"

	capabilityops "github.com/ahillspace/tadx/actions/capability"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Lister executes capability list.
type Lister interface {
	ListCapabilities(context.Context, capabilityops.ListInput) (capabilityops.ListOutput, error)
}

// Getter executes capability get.
type Getter interface {
	GetCapability(context.Context, capabilityops.GetInput) (capabilityops.GetOutput, error)
}

// Renderer writes structured action output.
type Renderer interface {
	Render(any) error
}

// Dependencies contains this domain's narrow wiring.
type Dependencies struct {
	Lister    Lister
	Getter    Getter
	Renderer  Renderer
	ListUse   string
	ListShort string
	GetUse    string
	GetShort  string
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
			input := capabilityops.ListInput{
				Environment: alias,
				Domain:      domain, Resource: resource, Owner: owner, Product: product,
				Mutation: mutationFilter, All: all, Cursor: cursor, Limit: limit,
				Full: full, JSON: jsonOutput,
			}
			output, err := deps.Lister.ListCapabilities(command.Context(), input)
			if err != nil {
				if output.MutationPolicy == "unavailable" {
					return clierr.WithOutput(output, err)
				}
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
			alias, _ := command.Flags().GetString("environment")
			output, err := deps.Getter.GetCapability(command.Context(), capabilityops.GetInput{ID: args[0], Environment: alias})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(output)
		},
	}
}

const cliCapabilityAnnotation = "tadx.capability"
