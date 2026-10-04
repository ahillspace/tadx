package app

import (
	"context"

	contentlabel "github.com/ahillspace/tadx/actions/contentlabel"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
)

type contentLabellistService struct{ runtime *runtimeDependencies }

func (c contentLabellistService) Execute(ctx context.Context, input contentlabel.ListInput) (contentlabel.ListOutput, error) {
	if err := contentlabel.ValidateListInput(input); err != nil {
		return contentlabel.ListOutput{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return contentlabel.ListOutput{}, capabilitySetupError("content.label.list.setup", "content.label.list", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return contentlabel.List(ctx, provider, input)
}

type contentLabelinspectService struct{ runtime *runtimeDependencies }

func (c contentLabelinspectService) Execute(ctx context.Context, input contentlabel.InspectInput) (contentlabel.InspectOutput, error) {
	if err := contentlabel.ValidateInspectInput(input); err != nil {
		return contentlabel.InspectOutput{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return contentlabel.InspectOutput{}, capabilitySetupError("content.label.inspect.setup", "content.label.inspect", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return contentlabel.Inspect(ctx, provider, input)
}

type contentLabelupdateService struct{ runtime *runtimeDependencies }

func (c contentLabelupdateService) Execute(ctx context.Context, input contentlabel.UpdateInput, preview bool) (contentlabel.UpdateOutput, error) {
	if err := contentlabel.ValidateUpdateInput(input); err != nil {
		return contentlabel.UpdateOutput{}, err
	}
	if err := c.runtime.checkManagedCapability("admin.label.value.inspect"); err != nil {
		return contentlabel.UpdateOutput{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return contentlabel.UpdateOutput{}, capabilitySetupError("content.label.update.setup", "content.label.update", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return contentlabel.Update(ctx, provider, provider, input, preview)
}

type contentLabeldeleteService struct{ runtime *runtimeDependencies }

func (c contentLabeldeleteService) Execute(ctx context.Context, input contentlabel.DeleteInput, preview bool) (contentlabel.DeleteOutput, error) {
	if err := contentlabel.ValidateDeleteInput(input); err != nil {
		return contentlabel.DeleteOutput{}, err
	}
	if err := c.runtime.checkManagedCapability("admin.label.value.inspect"); err != nil {
		return contentlabel.DeleteOutput{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return contentlabel.DeleteOutput{}, capabilitySetupError("content.label.delete.setup", "content.label.delete", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return contentlabel.Delete(ctx, provider, provider, input, preview)
}

func contentLabelDependencies(r *runtimeDependencies) *contentcli.LabelDependencies {
	return &contentcli.LabelDependencies{Lister: contentLabellistService{r}, Inspector: contentLabelinspectService{r}, Updater: contentLabelupdateService{r}, Deleter: contentLabeldeleteService{r}}
}
