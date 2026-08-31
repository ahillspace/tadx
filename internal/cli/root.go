// Package cli contains thin Cobra plumbing for TADX.
package cli

import (
	"context"
	"fmt"
	"strings"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
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

type AuthChecker interface {
	Execute(context.Context, authcheck.Input) (authcheck.Output, error)
}
type CatalogSearcher interface {
	Execute(context.Context, catalogsearch.Input) (catalogsearch.Output, error)
}
type WorkbookPuller interface {
	Execute(context.Context, workbookpull.Input) (workbookpull.Output, error)
}
type WorkbookPublisher interface {
	Execute(context.Context, workbookpublish.Input, bool) (workbookpublish.Output, error)
}

// Renderer writes structured command output.
type Renderer interface {
	Render(any) error
}

// Dependencies contains the explicitly wired Phase 0 command dependencies.
type Dependencies struct {
	Lister               Lister
	Getter               Getter
	Renderer             Renderer
	MutationsEnabled     bool
	ListUse              string
	ListShort            string
	GetUse               string
	GetShort             string
	AuthChecker          AuthChecker
	CatalogSearcher      CatalogSearcher
	WorkbookPuller       WorkbookPuller
	WorkbookPublisher    WorkbookPublisher
	AuthUse              string
	AuthShort            string
	CatalogSearchUse     string
	CatalogSearchShort   string
	WorkbookPullUse      string
	WorkbookPullShort    string
	WorkbookPublishUse   string
	WorkbookPublishShort string
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
	if deps.AuthChecker != nil {
		root.AddCommand(authcli.New(authcli.Dependencies{Checker: deps.AuthChecker, Renderer: deps.Renderer, Use: deps.AuthUse, Short: deps.AuthShort}))
	}
	if deps.CatalogSearcher != nil {
		root.AddCommand(catalogcli.New(catalogcli.Dependencies{Searcher: deps.CatalogSearcher, Renderer: deps.Renderer, Use: deps.CatalogSearchUse, Short: deps.CatalogSearchShort}))
	}
	if deps.WorkbookPuller != nil && deps.WorkbookPublisher != nil {
		root.AddCommand(contentcli.New(contentcli.Dependencies{Puller: deps.WorkbookPuller, Publisher: deps.WorkbookPublisher, Renderer: deps.Renderer, MutationsEnabled: deps.MutationsEnabled, PullUse: deps.WorkbookPullUse, PullShort: deps.WorkbookPullShort, PublishUse: deps.WorkbookPublishUse, PublishShort: deps.WorkbookPublishShort}))
	}
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
		return clierr.Usage(command.CommandPath(), cause)
	})
	for _, child := range command.Commands() {
		setFlagErrorHandlers(child)
	}
}
