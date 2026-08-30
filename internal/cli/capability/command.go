// Package capability registers the capability command domain.
package capability

import (
	"context"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/errs"
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
	Lister           Lister
	Getter           Getter
	Renderer         Renderer
	MutationsEnabled bool
	ListUse          string
	ListShort        string
	GetUse           string
	GetShort         string
}

// New creates the capability command and registers its implemented operations.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{
		Use:   "capability",
		Short: "Discover TADX capabilities",
	}
	command.AddCommand(newList(deps), newGet(deps))
	return command
}

func newList(deps Dependencies) *cobra.Command {
	var domain string
	var resource string
	var owner string
	var product string
	var mutation bool
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
				return usageError("capability.list", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			var mutationFilter *bool
			if command.Flags().Changed("mutation") {
				mutationFilter = &mutation
			}
			output, err := deps.Lister.Execute(command.Context(), capabilitylist.Input{
				Domain: domain, Resource: resource, Owner: owner, Product: product,
				Mutation: mutationFilter, Cursor: cursor, Limit: limit, MutationsEnabled: deps.MutationsEnabled,
			})
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
	command.Flags().StringVar(&cursor, "cursor", "", "continue from a prior result cursor")
	command.Flags().IntVar(&limit, "limit", capabilitylist.DefaultLimit, "maximum capabilities to return")
	return command
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
				return usageError("capability.get", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			output, err := deps.Getter.Execute(command.Context(), capabilityget.Input{ID: args[0]})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(output)
		},
	}
}

const cliCapabilityAnnotation = "tadx.capability"

func usageError(operation string, cause error) error {
	return &errs.Error{Kind: errs.KindUsage, Operation: operation, Summary: cause.Error(), Cause: cause}
}
