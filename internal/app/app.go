// Package app is the TADX composition root.
package app

import (
	"context"
	"errors"
	"io"
	"strings"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

// Options contains process-level discovery settings.
type Options struct {
	MutationsEnabled bool
}

// Run wires and runs the CLI, renders structured output, and returns an AXI exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, options Options) int {
	definitions := capability.All()
	source := registrySource{}
	root := cli.NewRoot(cli.Dependencies{
		Lister:           capabilitylist.New(source),
		Getter:           capabilityget.New(source),
		Renderer:         writerRenderer{writer: stdout},
		MutationsEnabled: options.MutationsEnabled,
		ListUse:          registryUse("capability.list"),
		ListShort:        registryShort("capability.list"),
		GetUse:           registryUse("capability.get"),
		GetShort:         registryShort("capability.get"),
	})
	registrations, err := cli.RegisteredCommands(root)
	if err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "CLI command registration validation failed.", Cause: err})
	}
	bindings := make([]capability.Binding, len(registrations))
	for index, registration := range registrations {
		bindings[index] = capability.Binding{CapabilityID: registration.CapabilityID, CommandPath: registration.CommandPath}
	}
	if err := capability.ValidateBindings(definitions, bindings); err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "Capability registry validation failed.", Cause: err})
	}
	root.SetOut(stdout)
	root.SetArgs(args)
	if _, _, err := root.Find(args); err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindUsage, Operation: "cli", Summary: err.Error(), Cause: err})
	}
	if err := root.ExecuteContext(ctx); err != nil {
		var structured *errs.Error
		if !errors.As(err, &structured) {
			err = &errs.Error{Kind: errs.KindRuntime, Operation: "cli", Summary: err.Error(), Cause: err}
		}
		return renderError(stdout, err)
	}
	return 0
}

func renderError(writer io.Writer, err error) int {
	if renderErr := output.RenderError(writer, err, output.Options{}); renderErr != nil {
		return 1
	}
	return errs.ExitCode(err)
}

type writerRenderer struct {
	writer io.Writer
}

func (r writerRenderer) Render(value any) error {
	return output.Render(r.writer, value)
}

type registrySource struct{}

func (registrySource) List(_ context.Context, includeMutations bool) ([]capabilitylist.Capability, error) {
	definitions := capability.All()
	items := make([]capabilitylist.Capability, 0, len(definitions))
	for _, definition := range definitions {
		if definition.RemoteMutation && !includeMutations {
			continue
		}
		items = append(items, capabilitylist.Capability{
			ID:             definition.ID,
			Owner:          string(definition.Owner),
			Disposition:    string(definition.Disposition),
			State:          string(definition.Implementation),
			Command:        strings.Join(definition.CommandPath, " "),
			Blocked:        definition.Verification == capability.VerificationBlocked,
			Domain:         filterDomain(definition),
			Resource:       filterResource(definition),
			Product:        definition.Availability,
			RemoteMutation: definition.RemoteMutation,
		})
	}
	return items, nil
}

func (registrySource) Get(_ context.Context, id string) (capabilityget.Capability, bool) {
	definition, ok := capability.Lookup(id)
	if !ok {
		return capabilityget.Capability{}, false
	}
	parts := strings.Split(definition.ID, ".")
	domain, resource := classify(definition)
	return capabilityget.Capability{
		ID:                    definition.ID,
		Domain:                domain,
		Resource:              resource,
		Verb:                  parts[len(parts)-1],
		Owner:                 string(definition.Owner),
		Surface:               definition.Surface,
		Outcome:               definition.Outcome,
		OperationType:         string(definition.Type),
		Disposition:           string(definition.Disposition),
		MCPOverlap:            definition.MCPOverlap,
		EvidenceLevel:         string(definition.EvidenceLevel),
		VerificationReadiness: string(definition.Verification),
		ImplementationState:   string(definition.Implementation),
		Command:               strings.Join(definition.CommandPath, " "),
		Selectors:             []string{definition.Selectors},
		Availability:          definition.Availability,
		SafetyGuard:           definition.SafetyGuard,
		ArtifactEffect:        definition.ArtifactEffect,
		UpstreamOperation:     definition.Upstream,
		Evidence:              definition.Evidence,
		Validation:            definition.Validation,
		Blocker:               string(definition.Blocker),
		RemoteMutation:        definition.RemoteMutation,
		RequiresApply:         definition.RequiresApply,
		LocalWrite:            definition.LocalWrite,
		RawCapable:            definition.RawCapable,
	}, true
}

func registryUse(id string) string {
	definition, ok := capability.Lookup(id)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(definition.Surface, "tadx capability ")
}

func registryShort(id string) string {
	definition, ok := capability.Lookup(id)
	if !ok {
		return ""
	}
	return definition.Outcome
}

func filterDomain(definition capability.Definition) string {
	domain, _ := classify(definition)
	return domain
}

func filterResource(definition capability.Definition) string {
	_, resource := classify(definition)
	return resource
}

func classify(definition capability.Definition) (string, string) {
	parts := strings.Split(definition.ID, ".")
	if definition.Owner == capability.OwnerCLI && (parts[0] == "workbook" || parts[0] == "datasource" || parts[0] == "flow" || parts[0] == "project") {
		return "content", parts[0]
	}
	if len(parts) == 3 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}
