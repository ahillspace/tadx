package admin

import (
	"context"
	"errors"
	categorycreate "github.com/ahillspace/tadx/actions/admin/labelcategory/create"
	categorydelete "github.com/ahillspace/tadx/actions/admin/labelcategory/delete"
	categoryinspect "github.com/ahillspace/tadx/actions/admin/labelcategory/inspect"
	categorylist "github.com/ahillspace/tadx/actions/admin/labelcategory/list"
	categoryupdate "github.com/ahillspace/tadx/actions/admin/labelcategory/update"
	valuedelete "github.com/ahillspace/tadx/actions/admin/labelvalue/delete"
	valueinspect "github.com/ahillspace/tadx/actions/admin/labelvalue/inspect"
	valuelist "github.com/ahillspace/tadx/actions/admin/labelvalue/list"
	valueupdate "github.com/ahillspace/tadx/actions/admin/labelvalue/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type LabelValueLister interface {
	Execute(context.Context, valuelist.Input) (valuelist.Output, error)
}
type LabelValueInspector interface {
	Execute(context.Context, valueinspect.Input) (valueinspect.Output, error)
}
type LabelValueUpdater interface {
	Execute(context.Context, valueupdate.Input, bool) (valueupdate.Output, error)
}
type LabelValueDeleter interface {
	Execute(context.Context, valuedelete.Input, bool) (valuedelete.Output, error)
}
type LabelCategoryLister interface {
	Execute(context.Context, categorylist.Input) (categorylist.Output, error)
}
type LabelCategoryInspector interface {
	Execute(context.Context, categoryinspect.Input) (categoryinspect.Output, error)
}
type LabelCategoryCreator interface {
	Execute(context.Context, categorycreate.Input, bool) (categorycreate.Output, error)
}
type LabelCategoryUpdater interface {
	Execute(context.Context, categoryupdate.Input, bool) (categoryupdate.Output, error)
}
type LabelCategoryDeleter interface {
	Execute(context.Context, categorydelete.Input, bool) (categorydelete.Output, error)
}
type LabelDependencies struct {
	ValueLister       LabelValueLister
	ValueInspector    LabelValueInspector
	ValueUpdater      LabelValueUpdater
	ValueDeleter      LabelValueDeleter
	CategoryLister    LabelCategoryLister
	CategoryInspector LabelCategoryInspector
	CategoryCreator   LabelCategoryCreator
	CategoryUpdater   LabelCategoryUpdater
	CategoryDeleter   LabelCategoryDeleter
	Renderer          Renderer
}

func NewLabels(deps LabelDependencies) *cobra.Command {
	root := &cobra.Command{Use: "label", Short: "Manage shared Tableau label values and categories"}
	values := &cobra.Command{Use: "value", Short: "Manage exact shared label value definitions"}
	categories := &cobra.Command{Use: "category", Short: "Manage exact shared label categories"}
	root.AddCommand(values, categories)
	values.AddCommand(newLabelValueLister(deps))
	values.AddCommand(newLabelValueInspector(deps))
	values.AddCommand(newLabelValueUpdater(deps))
	values.AddCommand(newLabelValueDeleter(deps))
	categories.AddCommand(newLabelCategoryLister(deps))
	categories.AddCommand(newLabelCategoryInspector(deps))
	categories.AddCommand(newLabelCategoryCreator(deps))
	categories.AddCommand(newLabelCategoryUpdater(deps))
	categories.AddCommand(newLabelCategoryDeleter(deps))
	return root
}
func newLabelValueLister(deps LabelDependencies) *cobra.Command {
	var in valuelist.Input
	cmd := &cobra.Command{Use: "list", Short: "List shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.list"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.list", err)
		}
		return valuelist.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueLister == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.list", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueLister.Execute(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().IntVar(&in.Limit, "limit", 20, "maximum returned records (1-10000)")
	return cmd
}
func newLabelValueInspector(deps LabelDependencies) *cobra.Command {
	var in valueinspect.Input
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.inspect", err)
		}
		return valueinspect.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueInspector == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.inspect", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueInspector.Execute(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	return cmd
}
func newLabelValueUpdater(deps LabelDependencies) *cobra.Command {
	var in valueupdate.Input
	var newname string
	var category string
	var description string
	var preview bool
	cmd := &cobra.Command{Use: "update", Short: "Update shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.update"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.update", err)
		}
		if cmd.Flags().Changed("new-name") {
			in.NewName = &newname
		}
		if cmd.Flags().Changed("category") {
			in.Category = &category
		}
		if cmd.Flags().Changed("description") {
			in.Description = &description
		}
		return valueupdate.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueUpdater == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.update", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueUpdater.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	cmd.Flags().StringVar(&newname, "new-name", "", "new exact definition name")
	cmd.Flags().StringVar(&category, "category", "", "category for a new label value; existing categories cannot change")
	cmd.Flags().StringVar(&description, "description", "", "shared definition description")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
func newLabelValueDeleter(deps LabelDependencies) *cobra.Command {
	var in valuedelete.Input
	var preview bool
	cmd := &cobra.Command{Use: "delete", Short: "Delete shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.delete"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.delete", err)
		}
		return valuedelete.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueDeleter == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.delete", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueDeleter.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
func newLabelCategoryLister(deps LabelDependencies) *cobra.Command {
	var in categorylist.Input
	cmd := &cobra.Command{Use: "list", Short: "List shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.list"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.list", err)
		}
		return categorylist.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryLister == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.list", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryLister.Execute(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().IntVar(&in.Limit, "limit", 20, "maximum returned records (1-10000)")
	return cmd
}
func newLabelCategoryInspector(deps LabelDependencies) *cobra.Command {
	var in categoryinspect.Input
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.inspect", err)
		}
		return categoryinspect.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryInspector == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.inspect", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryInspector.Execute(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	return cmd
}
func newLabelCategoryCreator(deps LabelDependencies) *cobra.Command {
	var in categorycreate.Input
	var preview bool
	cmd := &cobra.Command{Use: "create", Short: "Create shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.create"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.create", err)
		}
		return categorycreate.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryCreator == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.create", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryCreator.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	cmd.Flags().StringVar(&in.Description, "description", "", "description of this shared category")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
func newLabelCategoryUpdater(deps LabelDependencies) *cobra.Command {
	var in categoryupdate.Input
	var newname string
	var description string
	var preview bool
	cmd := &cobra.Command{Use: "update", Short: "Update shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.update"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.update", err)
		}
		if cmd.Flags().Changed("new-name") {
			in.NewName = &newname
		}
		if cmd.Flags().Changed("description") {
			in.Description = &description
		}
		return categoryupdate.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryUpdater == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.update", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryUpdater.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	cmd.Flags().StringVar(&newname, "new-name", "", "new exact definition name")
	cmd.Flags().StringVar(&description, "description", "", "shared definition description")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
func newLabelCategoryDeleter(deps LabelDependencies) *cobra.Command {
	var in categorydelete.Input
	var preview bool
	cmd := &cobra.Command{Use: "delete", Short: "Delete shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.delete"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.delete", err)
		}
		return categorydelete.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryDeleter == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.delete", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryDeleter.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Name, "name", "", "exact shared definition name")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
