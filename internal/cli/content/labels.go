package content

import (
	"context"
	"errors"
	labeldelete "github.com/ahillspace/tadx/actions/contentlabel/delete"
	labelinspect "github.com/ahillspace/tadx/actions/contentlabel/inspect"
	labellist "github.com/ahillspace/tadx/actions/contentlabel/list"
	labelupdate "github.com/ahillspace/tadx/actions/contentlabel/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type LabelLister interface {
	Execute(context.Context, labellist.Input) (labellist.Output, error)
}
type LabelInspector interface {
	Execute(context.Context, labelinspect.Input) (labelinspect.Output, error)
}
type LabelUpdater interface {
	Execute(context.Context, labelupdate.Input, bool) (labelupdate.Output, error)
}
type LabelDeleter interface {
	Execute(context.Context, labeldelete.Input, bool) (labeldelete.Output, error)
}
type LabelDependencies struct {
	Lister    LabelLister
	Inspector LabelInspector
	Updater   LabelUpdater
	Deleter   LabelDeleter
	Renderer  Renderer
}

func NewLabels(deps LabelDependencies) *cobra.Command {
	root := &cobra.Command{Use: "label", Short: "Inspect and change labels attached to Tableau assets"}
	root.AddCommand(newLabelLister(deps))
	root.AddCommand(newLabelInspector(deps))
	root.AddCommand(newLabelUpdater(deps))
	root.AddCommand(newLabelDeleter(deps))
	return root
}
func newLabelLister(deps LabelDependencies) *cobra.Command {
	var in labellist.Input
	cmd := &cobra.Command{Use: "list", Short: "List asset labels", Annotations: map[string]string{"tadx.capability": "content.label.list"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("content.label.list", err)
		}
		return labellist.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.Lister == nil || deps.Renderer == nil {
			return clierr.Usage("content.label.list", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.Lister.Execute(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.Type, "type", "", "asset type: database, table, column, datasource, or flow")
	cmd.Flags().StringVar(&in.TargetID, "target-id", "", "related asset REST LUID, not its Metadata API ID")
	cmd.Flags().StringArrayVar(&in.Categories, "category", nil, "exact label category; repeat for multiple categories")
	cmd.Flags().IntVar(&in.Limit, "limit", 0, "maximum returned records (1-10000, default 20)")
	cmd.Flags().BoolVar(&in.All, "all", false, "return all records within the 10000-record bound")
	cmd.MarkFlagsMutuallyExclusive("all", "limit")
	return cmd
}
func newLabelInspector(deps LabelDependencies) *cobra.Command {
	var in labelinspect.Input
	cmd := &cobra.Command{Use: "inspect", Short: "Inspect asset labels", Annotations: map[string]string{"tadx.capability": "content.label.inspect"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("content.label.inspect", err)
		}
		return labelinspect.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.Inspector == nil || deps.Renderer == nil {
			return clierr.Usage("content.label.inspect", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.Inspector.Execute(cmd.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.ID, "id", "", "exact label attachment LUID")
	cmd.Flags().StringVar(&in.Type, "type", "", "asset type: database, table, column, datasource, or flow")
	cmd.Flags().StringVar(&in.TargetID, "target-id", "", "related asset REST LUID, not its Metadata API ID")
	return cmd
}
func newLabelUpdater(deps LabelDependencies) *cobra.Command {
	var in labelupdate.Input
	var value string
	var message string
	var active bool
	var elevated bool
	var preview bool
	cmd := &cobra.Command{Use: "update", Short: "Update asset labels", Annotations: map[string]string{"tadx.capability": "content.label.update"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("content.label.update", err)
		}
		if cmd.Flags().Changed("value") {
			in.Value = &value
		}
		if cmd.Flags().Changed("message") {
			in.Message = &message
		}
		if cmd.Flags().Changed("active") {
			in.Active = &active
		}
		if cmd.Flags().Changed("elevated") {
			in.Elevated = &elevated
		}
		return labelupdate.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.Updater == nil || deps.Renderer == nil {
			return clierr.Usage("content.label.update", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.Updater.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.ID, "id", "", "exact label attachment LUID")
	cmd.Flags().StringVar(&in.Type, "type", "", "asset type: database, table, column, datasource, or flow")
	cmd.Flags().StringVar(&in.TargetID, "target-id", "", "related asset REST LUID, not its Metadata API ID")
	cmd.Flags().StringVar(&value, "value", "", "exact label value name")
	cmd.Flags().StringVar(&message, "message", "", "label message; an explicit empty value clears it")
	cmd.Flags().BoolVar(&active, "active", false, "whether this label is active")
	cmd.Flags().BoolVar(&elevated, "elevated", false, "whether this label has elevated visibility")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
func newLabelDeleter(deps LabelDependencies) *cobra.Command {
	var in labeldelete.Input
	var preview bool
	cmd := &cobra.Command{Use: "delete", Short: "Delete asset labels", Annotations: map[string]string{"tadx.capability": "content.label.delete"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("content.label.delete", err)
		}
		return labeldelete.ValidateInput(in)
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		if deps.Deleter == nil || deps.Renderer == nil {
			return clierr.Usage("content.label.delete", errors.New("label command dependencies are not configured"))
		}
		out, err := deps.Deleter.Execute(cmd.Context(), in, preview)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().StringVar(&in.Environment, "environment", "", "Tableau environment alias")
	cmd.Flags().StringVar(&in.ID, "id", "", "exact label attachment LUID")
	cmd.Flags().StringVar(&in.Type, "type", "", "asset type: database, table, column, datasource, or flow")
	cmd.Flags().StringVar(&in.TargetID, "target-id", "", "related asset REST LUID, not its Metadata API ID")
	cmd.Flags().BoolVar(&preview, "preview", false, "read the current state and show the plan without writing")
	return cmd
}
