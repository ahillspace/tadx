package app

import (
	"context"

	labelcategoryops "github.com/ahillspace/tadx/actions/admin/labelcategory"
	labelvalueops "github.com/ahillspace/tadx/actions/admin/labelvalue"
	contentlabel "github.com/ahillspace/tadx/actions/contentlabel"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
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

type adminLabelService struct{ runtime *runtimeDependencies }

func (c adminLabelService) ListLabelValue(ctx context.Context, input labelvalueops.ListInput) (labelvalueops.ListOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelvalueops.ListOutput{}, capabilitySetupError("admin.label.value.list.setup", "admin.label.value.list", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelvalueops.List(ctx, provider, input)
}

func (c adminLabelService) InspectLabelValue(ctx context.Context, input labelvalueops.InspectInput) (labelvalueops.InspectOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelvalueops.InspectOutput{}, capabilitySetupError("admin.label.value.inspect.setup", "admin.label.value.inspect", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelvalueops.Inspect(ctx, provider, input)
}

func (c adminLabelService) UpdateLabelValue(ctx context.Context, input labelvalueops.UpdateInput, preview bool) (labelvalueops.UpdateOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelvalueops.UpdateOutput{}, capabilitySetupError("admin.label.value.update.setup", "admin.label.value.update", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return labelvalueops.Update(ctx, provider, provider, input, preview)
}

func (c adminLabelService) DeleteLabelValue(ctx context.Context, input labelvalueops.DeleteInput, preview bool) (labelvalueops.DeleteOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelvalueops.DeleteOutput{}, capabilitySetupError("admin.label.value.delete.setup", "admin.label.value.delete", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return labelvalueops.Delete(ctx, provider, provider, input, preview)
}

func (c adminLabelService) ListLabelCategory(ctx context.Context, input labelcategoryops.ListInput) (labelcategoryops.ListOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelcategoryops.ListOutput{}, capabilitySetupError("admin.label.category.list.setup", "admin.label.category.list", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelcategoryops.List(ctx, provider, input)
}

func (c adminLabelService) InspectLabelCategory(ctx context.Context, input labelcategoryops.InspectInput) (labelcategoryops.InspectOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelcategoryops.InspectOutput{}, capabilitySetupError("admin.label.category.inspect.setup", "admin.label.category.inspect", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelcategoryops.Inspect(ctx, provider, input)
}

func (c adminLabelService) CreateLabelCategory(ctx context.Context, input labelcategoryops.CreateInput, preview bool) (labelcategoryops.WriteOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelcategoryops.WriteOutput{}, capabilitySetupError("admin.label.category.create.setup", "admin.label.category.create", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return labelcategoryops.Create(ctx, provider, provider, input, preview)
}

func (c adminLabelService) UpdateLabelCategory(ctx context.Context, input labelcategoryops.UpdateInput, preview bool) (labelcategoryops.WriteOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelcategoryops.WriteOutput{}, capabilitySetupError("admin.label.category.update.setup", "admin.label.category.update", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return labelcategoryops.Update(ctx, provider, provider, input, preview)
}

func (c adminLabelService) DeleteLabelCategory(ctx context.Context, input labelcategoryops.DeleteInput, preview bool) (labelcategoryops.DeleteOutput, error) {
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelcategoryops.DeleteOutput{}, capabilitySetupError("admin.label.category.delete.setup", "admin.label.category.delete", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	provider := c.runtime.clients(connection).metadataAssets
	return labelcategoryops.Delete(ctx, provider, provider, input, preview)
}
func contentLabelDependencies(r *runtimeDependencies) *contentcli.LabelDependencies {
	return &contentcli.LabelDependencies{Lister: contentLabellistService{r}, Inspector: contentLabelinspectService{r}, Updater: contentLabelupdateService{r}, Deleter: contentLabeldeleteService{r}}
}
func adminLabelDependencies(r *runtimeDependencies) *admincli.LabelDependencies {
	return &admincli.LabelDependencies{ValueLister: adminLabelService{r}, ValueInspector: adminLabelService{r}, ValueUpdater: adminLabelService{r}, ValueDeleter: adminLabelService{r}, CategoryLister: adminLabelService{r}, CategoryInspector: adminLabelService{r}, CategoryCreator: adminLabelService{r}, CategoryUpdater: adminLabelService{r}, CategoryDeleter: adminLabelService{r}}
}
