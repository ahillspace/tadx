package admin

import (
	"context"
	"errors"

	labelcategoryops "github.com/ahillspace/tadx/actions/admin/labelcategory"
	labelvalueops "github.com/ahillspace/tadx/actions/admin/labelvalue"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type LabelValueLister interface {
	ListLabelValue(context.Context, labelvalueops.ListInput) (labelvalueops.ListOutput, error)
}
type LabelValueInspector interface {
	InspectLabelValue(context.Context, labelvalueops.InspectInput) (labelvalueops.InspectOutput, error)
}
type LabelValueUpdater interface {
	UpdateLabelValue(context.Context, labelvalueops.UpdateInput, bool) (labelvalueops.UpdateOutput, error)
}
type LabelValueDeleter interface {
	DeleteLabelValue(context.Context, labelvalueops.DeleteInput, bool) (labelvalueops.DeleteOutput, error)
}
type LabelCategoryLister interface {
	ListLabelCategory(context.Context, labelcategoryops.ListInput) (labelcategoryops.ListOutput, error)
}
type LabelCategoryInspector interface {
	InspectLabelCategory(context.Context, labelcategoryops.InspectInput) (labelcategoryops.InspectOutput, error)
}
type LabelCategoryCreator interface {
	CreateLabelCategory(context.Context, labelcategoryops.CreateInput, bool) (labelcategoryops.WriteOutput, error)
}
type LabelCategoryUpdater interface {
	UpdateLabelCategory(context.Context, labelcategoryops.UpdateInput, bool) (labelcategoryops.WriteOutput, error)
}
type LabelCategoryDeleter interface {
	DeleteLabelCategory(context.Context, labelcategoryops.DeleteInput, bool) (labelcategoryops.DeleteOutput, error)
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

func NewLabels(deps LabelDependencies) []*cobra.Command {
	values := &cobra.Command{Use: "label-value", Short: "Manage exact shared label value definitions"}
	categories := &cobra.Command{Use: "label-category", Short: "Manage exact shared label categories"}
	values.AddCommand(newLabelValueLister(deps))
	values.AddCommand(newLabelValueInspector(deps))
	values.AddCommand(newLabelValueUpdater(deps))
	values.AddCommand(newLabelValueDeleter(deps))
	categories.AddCommand(newLabelCategoryLister(deps))
	categories.AddCommand(newLabelCategoryInspector(deps))
	categories.AddCommand(newLabelCategoryCreator(deps))
	categories.AddCommand(newLabelCategoryUpdater(deps))
	categories.AddCommand(newLabelCategoryDeleter(deps))
	return []*cobra.Command{values, categories}
}
func newLabelValueLister(deps LabelDependencies) *cobra.Command {
	var in labelvalueops.ListInput
	cmd := &cobra.Command{Use: "list", Short: "List shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.list"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.list", err)
		}
		return labelvalueops.ValidateListInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueLister == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.list", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueLister.ListLabelValue(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().IntVar(&in.Limit, "limit", 0, "maximum returned records (1-10000, default 20)")
	cmd.Flags().BoolVar(&in.All, "all", false, "return all records within the 10000-record bound")
	cmd.MarkFlagsMutuallyExclusive("all", "limit")
	return cmd
}
func newLabelValueInspector(deps LabelDependencies) *cobra.Command {
	var in labelvalueops.InspectInput
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.inspect", err)
		}
		return labelvalueops.ValidateInspectInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueInspector == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.inspect", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueInspector.InspectLabelValue(cmd.Context(), in)
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
	var in labelvalueops.UpdateInput
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
		return labelvalueops.ValidateUpdateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueUpdater == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.update", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueUpdater.UpdateLabelValue(cmd.Context(), in, preview)
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
	var in labelvalueops.DeleteInput
	var preview bool
	cmd := &cobra.Command{Use: "delete", Short: "Delete shared label value definitions", Annotations: map[string]string{"tadx.capability": "admin.label.value.delete"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.value.delete", err)
		}
		return labelvalueops.ValidateDeleteInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.ValueDeleter == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.value.delete", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.ValueDeleter.DeleteLabelValue(cmd.Context(), in, preview)
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
	var in labelcategoryops.ListInput
	cmd := &cobra.Command{Use: "list", Short: "List shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.list"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.list", err)
		}
		return labelcategoryops.ValidateListInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryLister == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.list", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryLister.ListLabelCategory(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().IntVar(&in.Limit, "limit", 0, "maximum returned records (1-10000, default 20)")
	cmd.Flags().BoolVar(&in.All, "all", false, "return all records within the 10000-record bound")
	cmd.MarkFlagsMutuallyExclusive("all", "limit")
	return cmd
}
func newLabelCategoryInspector(deps LabelDependencies) *cobra.Command {
	var in labelcategoryops.InspectInput
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.inspect", err)
		}
		return labelcategoryops.ValidateInspectInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryInspector == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.inspect", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryInspector.InspectLabelCategory(cmd.Context(), in)
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
	var in labelcategoryops.CreateInput
	var preview bool
	cmd := &cobra.Command{Use: "create", Short: "Create shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.create"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.create", err)
		}
		return labelcategoryops.ValidateCreateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryCreator == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.create", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryCreator.CreateLabelCategory(cmd.Context(), in, preview)
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
	var in labelcategoryops.UpdateInput
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
		return labelcategoryops.ValidateUpdateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryUpdater == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.update", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryUpdater.UpdateLabelCategory(cmd.Context(), in, preview)
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
	var in labelcategoryops.DeleteInput
	var preview bool
	cmd := &cobra.Command{Use: "delete", Short: "Delete shared label category definitions", Annotations: map[string]string{"tadx.capability": "admin.label.category.delete"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("admin.label.category.delete", err)
		}
		return labelcategoryops.ValidateDeleteInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.CategoryDeleter == nil || deps.Renderer == nil {
			return clierr.Usage("admin.label.category.delete", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.CategoryDeleter.DeleteLabelCategory(cmd.Context(), in, preview)
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
