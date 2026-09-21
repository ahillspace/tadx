package app

import (
	"context"
	labelCategorycreate "github.com/ahillspace/tadx/actions/admin/labelcategory/create"
	labelCategorydelete "github.com/ahillspace/tadx/actions/admin/labelcategory/delete"
	labelCategoryinspect "github.com/ahillspace/tadx/actions/admin/labelcategory/inspect"
	labelCategorylist "github.com/ahillspace/tadx/actions/admin/labelcategory/list"
	labelCategoryupdate "github.com/ahillspace/tadx/actions/admin/labelcategory/update"
	labelValuedelete "github.com/ahillspace/tadx/actions/admin/labelvalue/delete"
	labelValueinspect "github.com/ahillspace/tadx/actions/admin/labelvalue/inspect"
	labelValuelist "github.com/ahillspace/tadx/actions/admin/labelvalue/list"
	labelValueupdate "github.com/ahillspace/tadx/actions/admin/labelvalue/update"
	contentLabeldelete "github.com/ahillspace/tadx/actions/contentlabel/delete"
	contentLabelinspect "github.com/ahillspace/tadx/actions/contentlabel/inspect"
	contentLabellist "github.com/ahillspace/tadx/actions/contentlabel/list"
	contentLabelupdate "github.com/ahillspace/tadx/actions/contentlabel/update"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
)

type contentLabellistService struct{ runtime *runtimeDependencies }

func (c contentLabellistService) Execute(ctx context.Context, input contentLabellist.Input) (contentLabellist.Output, error) {
	if err := contentLabellist.ValidateInput(input); err != nil {
		return contentLabellist.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return contentLabellist.Output{}, capabilitySetupError("content.label.list.setup", "content.label.list", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return contentLabellist.New(provider).Execute(ctx, input)
}

type contentLabelinspectService struct{ runtime *runtimeDependencies }

func (c contentLabelinspectService) Execute(ctx context.Context, input contentLabelinspect.Input) (contentLabelinspect.Output, error) {
	if err := contentLabelinspect.ValidateInput(input); err != nil {
		return contentLabelinspect.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return contentLabelinspect.Output{}, capabilitySetupError("content.label.inspect.setup", "content.label.inspect", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return contentLabelinspect.New(provider).Execute(ctx, input)
}

type contentLabelupdateService struct{ runtime *runtimeDependencies }

func (c contentLabelupdateService) Execute(ctx context.Context, input contentLabelupdate.Input, preview bool) (contentLabelupdate.Output, error) {
	if err := contentLabelupdate.ValidateInput(input); err != nil {
		return contentLabelupdate.Output{}, err
	}
	if err := c.runtime.checkManagedCapability("admin.label.value.inspect"); err != nil {
		return contentLabelupdate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return contentLabelupdate.Output{}, capabilitySetupError("content.label.update.setup", "content.label.update", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return contentLabelupdate.New(provider, provider).Execute(ctx, input, preview)
}

type contentLabeldeleteService struct{ runtime *runtimeDependencies }

func (c contentLabeldeleteService) Execute(ctx context.Context, input contentLabeldelete.Input, preview bool) (contentLabeldelete.Output, error) {
	if err := contentLabeldelete.ValidateInput(input); err != nil {
		return contentLabeldelete.Output{}, err
	}
	if err := c.runtime.checkManagedCapability("admin.label.value.inspect"); err != nil {
		return contentLabeldelete.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return contentLabeldelete.Output{}, capabilitySetupError("content.label.delete.setup", "content.label.delete", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return contentLabeldelete.New(provider, provider).Execute(ctx, input, preview)
}

type labelValuelistService struct{ runtime *runtimeDependencies }

func (c labelValuelistService) Execute(ctx context.Context, input labelValuelist.Input) (labelValuelist.Output, error) {
	if err := labelValuelist.ValidateInput(input); err != nil {
		return labelValuelist.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelValuelist.Output{}, capabilitySetupError("admin.label.value.list.setup", "admin.label.value.list", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelValuelist.New(provider).Execute(ctx, input)
}

type labelValueinspectService struct{ runtime *runtimeDependencies }

func (c labelValueinspectService) Execute(ctx context.Context, input labelValueinspect.Input) (labelValueinspect.Output, error) {
	if err := labelValueinspect.ValidateInput(input); err != nil {
		return labelValueinspect.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelValueinspect.Output{}, capabilitySetupError("admin.label.value.inspect.setup", "admin.label.value.inspect", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelValueinspect.New(provider).Execute(ctx, input)
}

type labelValueupdateService struct{ runtime *runtimeDependencies }

func (c labelValueupdateService) Execute(ctx context.Context, input labelValueupdate.Input, preview bool) (labelValueupdate.Output, error) {
	if err := labelValueupdate.ValidateInput(input); err != nil {
		return labelValueupdate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelValueupdate.Output{}, capabilitySetupError("admin.label.value.update.setup", "admin.label.value.update", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return labelValueupdate.New(provider, provider).Execute(ctx, input, preview)
}

type labelValuedeleteService struct{ runtime *runtimeDependencies }

func (c labelValuedeleteService) Execute(ctx context.Context, input labelValuedelete.Input, preview bool) (labelValuedelete.Output, error) {
	if err := labelValuedelete.ValidateInput(input); err != nil {
		return labelValuedelete.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelValuedelete.Output{}, capabilitySetupError("admin.label.value.delete.setup", "admin.label.value.delete", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return labelValuedelete.New(provider, provider).Execute(ctx, input, preview)
}

type labelCategorylistService struct{ runtime *runtimeDependencies }

func (c labelCategorylistService) Execute(ctx context.Context, input labelCategorylist.Input) (labelCategorylist.Output, error) {
	if err := labelCategorylist.ValidateInput(input); err != nil {
		return labelCategorylist.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelCategorylist.Output{}, capabilitySetupError("admin.label.category.list.setup", "admin.label.category.list", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelCategorylist.New(provider).Execute(ctx, input)
}

type labelCategoryinspectService struct{ runtime *runtimeDependencies }

func (c labelCategoryinspectService) Execute(ctx context.Context, input labelCategoryinspect.Input) (labelCategoryinspect.Output, error) {
	if err := labelCategoryinspect.ValidateInput(input); err != nil {
		return labelCategoryinspect.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return labelCategoryinspect.Output{}, capabilitySetupError("admin.label.category.inspect.setup", "admin.label.category.inspect", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	provider := c.runtime.clients(connection).metadataAssets
	return labelCategoryinspect.New(provider).Execute(ctx, input)
}

type labelCategorycreateService struct{ runtime *runtimeDependencies }

func (c labelCategorycreateService) Execute(ctx context.Context, input labelCategorycreate.Input, preview bool) (labelCategorycreate.Output, error) {
	if err := labelCategorycreate.ValidateInput(input); err != nil {
		return labelCategorycreate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelCategorycreate.Output{}, capabilitySetupError("admin.label.category.create.setup", "admin.label.category.create", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return labelCategorycreate.New(provider, provider).Execute(ctx, input, preview)
}

type labelCategoryupdateService struct{ runtime *runtimeDependencies }

func (c labelCategoryupdateService) Execute(ctx context.Context, input labelCategoryupdate.Input, preview bool) (labelCategoryupdate.Output, error) {
	if err := labelCategoryupdate.ValidateInput(input); err != nil {
		return labelCategoryupdate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelCategoryupdate.Output{}, capabilitySetupError("admin.label.category.update.setup", "admin.label.category.update", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return labelCategoryupdate.New(provider, provider).Execute(ctx, input, preview)
}

type labelCategorydeleteService struct{ runtime *runtimeDependencies }

func (c labelCategorydeleteService) Execute(ctx context.Context, input labelCategorydelete.Input, preview bool) (labelCategorydelete.Output, error) {
	if err := labelCategorydelete.ValidateInput(input); err != nil {
		return labelCategorydelete.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return labelCategorydelete.Output{}, capabilitySetupError("admin.label.category.delete.setup", "admin.label.category.delete", input.Environment, connection.environment.SiteContentURL, "Label operation setup failed.", "Verify the target environment and Tableau label access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	provider := c.runtime.clients(connection).metadataAssets
	return labelCategorydelete.New(provider, provider).Execute(ctx, input, preview)
}
func contentLabelDependencies(r *runtimeDependencies) *contentcli.LabelDependencies {
	return &contentcli.LabelDependencies{Lister: contentLabellistService{r}, Inspector: contentLabelinspectService{r}, Updater: contentLabelupdateService{r}, Deleter: contentLabeldeleteService{r}}
}
func adminLabelDependencies(r *runtimeDependencies) *admincli.LabelDependencies {
	return &admincli.LabelDependencies{ValueLister: labelValuelistService{r}, ValueInspector: labelValueinspectService{r}, ValueUpdater: labelValueupdateService{r}, ValueDeleter: labelValuedeleteService{r}, CategoryLister: labelCategorylistService{r}, CategoryInspector: labelCategoryinspectService{r}, CategoryCreator: labelCategorycreateService{r}, CategoryUpdater: labelCategoryupdateService{r}, CategoryDeleter: labelCategorydeleteService{r}}
}
