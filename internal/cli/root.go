// Package cli contains thin Cobra plumbing for TADX.
package cli

import (
	"context"
	"fmt"
	"strings"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	doctorcli "github.com/ahillspace/tadx/internal/cli/doctor"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
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
type AuthStatuser interface {
	Execute(context.Context, authstatus.Input) (authstatus.Output, error)
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

// RenderOptions contains presentation-only flags shared by every command.
type RenderOptions struct {
	Full bool
}

// Dependencies contains the explicitly wired Phase 0 command dependencies.
type Dependencies struct {
	Lister               Lister
	Getter               Getter
	Renderer             Renderer
	RenderOptions        *RenderOptions
	ConfigPath           *string
	MutationsEnabled     bool
	ListUse              string
	ListShort            string
	GetUse               string
	GetShort             string
	AuthChecker          AuthChecker
	CatalogSearcher      CatalogSearcher
	CatalogRefresher     catalogcli.Refresher
	CatalogGetter        catalogcli.Getter
	CatalogStatuser      catalogcli.Statuser
	WorkbookPuller       WorkbookPuller
	WorkbookPublisher    WorkbookPublisher
	Content              *contentcli.Dependencies
	EnvironmentProfiles  *envcli.Dependencies
	Workspaces           *workspacecli.Dependencies
	Admin                *admincli.Dependencies
	DoctorRunner         doctorcli.Runner
	DoctorUse            string
	DoctorShort          string
	AuthUse              string
	AuthShort            string
	AuthStatuser         AuthStatuser
	AuthStatusUse        string
	AuthStatusShort      string
	CatalogSearchUse     string
	CatalogSearchShort   string
	CatalogRefreshUse    string
	CatalogRefreshShort  string
	CatalogGetUse        string
	CatalogGetShort      string
	CatalogStatusUse     string
	CatalogStatusShort   string
	WorkbookPullUse      string
	WorkbookPullShort    string
	WorkbookPublishUse   string
	WorkbookPublishShort string
}

// NewRoot creates the root command. It contains no domain behavior.
func NewRoot(deps Dependencies) *cobra.Command {
	renderOptions := deps.RenderOptions
	if renderOptions == nil {
		renderOptions = &RenderOptions{}
	}
	configPath := deps.ConfigPath
	if configPath == nil {
		configPath = new(string)
	}
	root := &cobra.Command{
		Use:           "tadx",
		Short:         "Deterministic Tableau lifecycle and development CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().BoolVar(&renderOptions.Full, "full", false, "show expanded bounded details")
	root.PersistentFlags().StringVar(configPath, "config", *configPath, "path to the non-secret TADX configuration file")
	root.PersistentFlags().Lookup("config").DefValue = ""
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
	if deps.EnvironmentProfiles != nil {
		environmentProfiles := *deps.EnvironmentProfiles
		environmentProfiles.Renderer = deps.Renderer
		root.AddCommand(envcli.New(environmentProfiles))
	}
	if deps.Workspaces != nil {
		workspaces := *deps.Workspaces
		workspaces.Renderer = deps.Renderer
		root.AddCommand(workspacecli.New(workspaces))
	}
	if deps.Admin != nil {
		admin := *deps.Admin
		admin.Renderer = deps.Renderer
		admin.MutationsEnabled = deps.MutationsEnabled
		root.AddCommand(admincli.New(admin))
	}
	if deps.DoctorRunner != nil {
		root.AddCommand(doctorcli.New(doctorcli.Dependencies{Runner: deps.DoctorRunner, Renderer: deps.Renderer, Use: deps.DoctorUse, Short: deps.DoctorShort}))
	}
	if deps.AuthChecker != nil {
		root.AddCommand(authcli.New(authcli.Dependencies{Checker: deps.AuthChecker, Statuser: deps.AuthStatuser, Renderer: deps.Renderer, Use: deps.AuthUse, Short: deps.AuthShort, StatusUse: deps.AuthStatusUse, StatusShort: deps.AuthStatusShort}))
	}
	if deps.CatalogSearcher != nil {
		root.AddCommand(catalogcli.New(catalogcli.Dependencies{
			Searcher: deps.CatalogSearcher, Refresher: deps.CatalogRefresher, Getter: deps.CatalogGetter, Statuser: deps.CatalogStatuser,
			Renderer: deps.Renderer, Use: deps.CatalogSearchUse, Short: deps.CatalogSearchShort,
			RefreshUse: deps.CatalogRefreshUse, RefreshShort: deps.CatalogRefreshShort,
			GetUse: deps.CatalogGetUse, GetShort: deps.CatalogGetShort,
			StatusUse: deps.CatalogStatusUse, StatusShort: deps.CatalogStatusShort,
		}))
	}
	if deps.WorkbookPuller != nil && deps.WorkbookPublisher != nil {
		contentDependencies := contentcli.Dependencies{}
		if deps.Content != nil {
			contentDependencies = *deps.Content
		}
		contentDependencies.Puller = deps.WorkbookPuller
		contentDependencies.Publisher = deps.WorkbookPublisher
		contentDependencies.Renderer = deps.Renderer
		contentDependencies.MutationsEnabled = deps.MutationsEnabled
		contentDependencies.PullUse = deps.WorkbookPullUse
		contentDependencies.PullShort = deps.WorkbookPullShort
		contentDependencies.PublishUse = deps.WorkbookPublishUse
		contentDependencies.PublishShort = deps.WorkbookPublishShort
		root.AddCommand(contentcli.New(contentDependencies))
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
