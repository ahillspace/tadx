package content

import (
	"context"
	"errors"
	"strings"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/commandset"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

type EditFiles struct {
	Workspace, FromChanges, AuditFile, ChangesFile string
	TemplateFields                                 []string
}
type CheckoutOptions struct {
	EditFiles
	Environment, ID, Name, Project, ProjectID string
	Acknowledge                               []string
	Preview                                   bool
}
type InspectionOptions struct {
	EditFiles
	Environment, SourceID, WorkCopy, Attempt string
	ListAttempts, Verify                     bool
	Fields, Tables                           []string
}
type ApplyOptions struct {
	EditFiles
	Environment, WorkCopy, ExpectPreview string
	Change                               ds.EditChange
	Preview                              bool
}
type SaveOptions struct {
	EditFiles
	Environment, Mode, WorkCopy, Name, ExpectPreview string
	Description                                      *string
	Fields, Acknowledge                              []string
	Preview                                          bool
}
type DatasourceEditor interface {
	CheckoutDatasource(context.Context, CheckoutOptions) (any, error)
	InspectElements(context.Context, InspectionOptions) (any, error)
	ApplyEdits(context.Context, ApplyOptions) (any, error)
	ValidateChanges(context.Context, EditFiles) (any, error)
	SaveCopy(context.Context, SaveOptions) (any, error)
	ChangeTag(context.Context, ds.TagInput, bool) (ds.TagOutput, error)
}

func environmentFlag(cmd *cobra.Command, env *string) {
	cmd.Flags().StringVar(env, "environment", "", "configured Tableau environment alias")
}
func previewFlag(cmd *cobra.Command, preview *bool) {
	cmd.Flags().BoolVar(preview, "preview", false, "show live observations and the proposed operation without submitting it")
}
func workspaceFlag(cmd *cobra.Command, files *EditFiles) {
	cmd.Flags().StringVar(&files.Workspace, "workspace", "", "workspace containing the input or output files")
}
func inputFileFlag(cmd *cobra.Command, files *EditFiles) {
	cmd.Flags().StringVar(&files.FromChanges, "from-changes", "", "workspace-relative typed changes YAML")
}
func outputFileFlags(cmd *cobra.Command, files *EditFiles) {
	cmd.Flags().StringVar(&files.AuditFile, "audit-file", "", "new workspace-relative Markdown audit path")
	cmd.Flags().StringVar(&files.ChangesFile, "changes-file", "", "new workspace-relative YAML starter path")
	cmd.Flags().StringArrayVar(&files.TemplateFields, "template-field", nil, "exact field to include as a commented starter example (repeatable)")
}
func workCopyFlag(cmd *cobra.Command, id *string) {
	cmd.Flags().StringVar(id, "work-copy", "", "exact work-copy LUID")
}
func tokenFlag(cmd *cobra.Command, token *string) {
	cmd.Args = func(c *cobra.Command, args []string) error {
		if err := cobra.NoArgs(c, args); err != nil {
			return err
		}
		if c.Flags().Changed("expect-preview") && *token == "" {
			return clierr.Usage(c.CommandPath(), errors.New("--expect-preview requires a nonempty comparison token"))
		}
		return nil
	}
	cmd.Flags().StringVar(token, "expect-preview", "", "require the live comparison to match this preview token")
}
func ackFlag(cmd *cobra.Command, ids *[]string) {
	cmd.Flags().StringArrayVar(ids, "acknowledge-unknown", nil, "accept the risk of a new submission for this exact unresolved attempt (repeatable)")
}

func addDatasourceEditCommands(parent *cobra.Command, d Dependencies, sel commandset.Selection) {
	editor := d.DatasourceEditor
	// Unqualified leaves retain argument help. Their only code path returns the
	// evidence limitation; they have no action binding or authenticated dependency.
	mount := func(p *cobra.Command, id, summary string, bind func(*cobra.Command) func(context.Context) (any, error)) {
		sel.Add(p, id, func() *cobra.Command {
			parts := strings.Split(id, ".")
			c := &cobra.Command{Use: parts[len(parts)-1], Short: summary, Args: cobra.NoArgs}
			execute := bind(c)
			if reason := d.DatasourceEditBlocker(id); reason != "" {
				c.Hidden = true
				c.Short += " (awaiting provider verification)"
				c.Long = summary + "\n\n" + reason
				c.Annotations = map[string]string{"tadx.grouping": "true", "tadx.evidence-required": reason}
				c.RunE = func(*cobra.Command, []string) error {
					return &errs.Error{ID: "datasource.evidence_required", Operation: id, Kind: errs.KindOperation, Summary: reason, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: new(false), CorrectiveAction: "Inspect tadx capability get " + id + " --full for qualification requirements."}
				}
				return c
			}
			c.Annotations = map[string]string{"tadx.capability": id}
			c.RunE = func(c *cobra.Command, _ []string) error {
				value, err := execute(c.Context())
				if err != nil {
					return clierr.WithOutput(value, err)
				}
				return d.Renderer.Render(value)
			}
			return c
		})
	}
	mount(parent, "datasource.validate-changes", "Validate local edit YAML.", func(c *cobra.Command) func(context.Context) (any, error) {
		var in EditFiles
		workspaceFlag(c, &in)
		inputFileFlag(c, &in)
		_ = c.MarkFlagRequired("from-changes")
		return func(ctx context.Context) (any, error) { return editor.ValidateChanges(ctx, in) }
	})
	mount(parent, "datasource.checkout", "Create a server-side work copy of one published datasource.", func(c *cobra.Command) func(context.Context) (any, error) {
		var in CheckoutOptions
		environmentFlag(c, &in.Environment)
		previewFlag(c, &in.Preview)
		workspaceFlag(c, &in.EditFiles)
		outputFileFlags(c, &in.EditFiles)
		ackFlag(c, &in.Acknowledge)
		c.Flags().StringVar(&in.ID, "id", "", "exact published datasource LUID")
		c.Flags().StringVar(&in.Name, "name", "", "exact published datasource name")
		c.Flags().StringVar(&in.Project, "project", "", "exact project path")
		c.Flags().StringVar(&in.ProjectID, "project-id", "", "exact project LUID")
		c.MarkFlagsOneRequired("id", "name")
		c.MarkFlagsMutuallyExclusive("id", "name")
		c.MarkFlagsMutuallyExclusive("project", "project-id")
		return func(ctx context.Context) (any, error) { return editor.CheckoutDatasource(ctx, in) }
	})
	for _, kind := range []string{"elements", "work-copy"} {
		summary := "Inspect current TDS columns and descriptions on a published datasource."
		if kind == "work-copy" {
			summary = "Inspect, export or review an exact work copy and recover scoped attempts."
		}
		mount(parent, "datasource."+kind, summary, func(c *cobra.Command) func(context.Context) (any, error) {
			var in InspectionOptions
			environmentFlag(c, &in.Environment)
			c.Flags().StringArrayVar(&in.Fields, "field", nil, "read an exact TDS field name (repeatable)")
			c.Flags().StringArrayVar(&in.Tables, "table", nil, "read an exact TDS table description (repeatable)")
			c.Flags().StringVar(&in.SourceID, "id", "", "exact published datasource LUID")
			if kind == "elements" {
				_ = c.MarkFlagRequired("id")
			} else {
				workspaceFlag(c, &in.EditFiles)
				inputFileFlag(c, &in.EditFiles)
				outputFileFlags(c, &in.EditFiles)
				workCopyFlag(c, &in.WorkCopy)
				c.Flags().StringVar(&in.Attempt, "attempt", "", "scoped checkout or save attempt ID")
				c.Flags().BoolVar(&in.ListAttempts, "attempts", false, "list recent scoped attempts")
				c.Flags().BoolVar(&in.Verify, "verify", false, "compare a saved attempt with its published projection")
			}
			return func(ctx context.Context) (any, error) {
				if kind == "work-copy" && in.SourceID != "" && !in.ListAttempts {
					return nil, clierr.Usage("datasource.work-copy", errors.New("--id filters --attempts"))
				}
				return editor.InspectElements(ctx, in)
			}
		})
	}
	for _, op := range []string{"set-field-description", "remove-field-description", "set-table-description", "remove-table-description", "apply-changes"} {
		summary := "Edit one exact work-copy description and verify its value."
		if op == "apply-changes" {
			summary = "Apply up to 50 typed work-copy edits in file order with independent readback."
		}
		mount(parent, "datasource."+op, summary, func(c *cobra.Command) func(context.Context) (any, error) {
			var in ApplyOptions
			environmentFlag(c, &in.Environment)
			previewFlag(c, &in.Preview)
			tokenFlag(c, &in.ExpectPreview)
			if op == "apply-changes" {
				workspaceFlag(c, &in.EditFiles)
				inputFileFlag(c, &in.EditFiles)
				_ = c.MarkFlagRequired("from-changes")
			} else {
				workCopyFlag(c, &in.WorkCopy)
				_ = c.MarkFlagRequired("work-copy")
				in.Change.Op = op
				if strings.Contains(op, "field") {
					c.Flags().StringVar(&in.Change.Field, "field", "", "exact TDS field name")
					_ = c.MarkFlagRequired("field")
				} else {
					c.Flags().StringVar(&in.Change.Table, "table", "", "exact TDS table name")
					_ = c.MarkFlagRequired("table")
				}
				if strings.HasPrefix(op, "set-") {
					c.Flags().StringVar(&in.Change.Description, "description", "", "new description text")
					_ = c.MarkFlagRequired("description")
				}
			}
			return func(ctx context.Context) (any, error) { return editor.ApplyEdits(ctx, in) }
		})
	}
	if sel.Wants("datasource.save") {
		save := &cobra.Command{Use: "save", Short: "Publish a reviewed work copy to an explicit destination"}
		save.Hidden = d.DatasourceEditBlocker("datasource.save.as-new") != "" && d.DatasourceEditBlocker("datasource.save.overwrite") != ""
		parent.AddCommand(save)
		for _, mode := range []string{"overwrite", "as-new"} {
			summary := "Replace the verified original datasource from its work copy."
			if mode == "as-new" {
				summary = "Create a new published datasource from its work copy."
			}
			mount(save, "datasource.save."+mode, summary, func(c *cobra.Command) func(context.Context) (any, error) {
				in := SaveOptions{Mode: mode}
				var desc string
				environmentFlag(c, &in.Environment)
				previewFlag(c, &in.Preview)
				tokenFlag(c, &in.ExpectPreview)
				ackFlag(c, &in.Acknowledge)
				workspaceFlag(c, &in.EditFiles)
				inputFileFlag(c, &in.EditFiles)
				workCopyFlag(c, &in.WorkCopy)
				c.MarkFlagsOneRequired("work-copy", "from-changes")
				c.MarkFlagsMutuallyExclusive("work-copy", "from-changes")
				c.Flags().StringArrayVar(&in.Fields, "field", nil, "verify the exact field's published description (repeatable)")
				c.MarkFlagsMutuallyExclusive("field", "from-changes")
				if mode == "as-new" {
					c.Flags().StringVar(&in.Name, "name", "", "new published datasource name")
					_ = c.MarkFlagRequired("name")
					c.Flags().StringVar(&desc, "description", "", "description for the new published datasource")
				}
				return func(ctx context.Context) (any, error) {
					if c.Flags().Changed("description") {
						in.Description = &desc
					}
					return editor.SaveCopy(ctx, in)
				}
			})
		}
	}
	for _, op := range []string{"add-tag", "remove-tag"} {
		mount(parent, "datasource."+op, "Change one tag on the published datasource and verify membership.", func(c *cobra.Command) func(context.Context) (any, error) {
			in := ds.TagInput{Add: op == "add-tag"}
			var preview bool
			environmentFlag(c, &in.Environment)
			previewFlag(c, &preview)
			c.Flags().StringVar(&in.DatasourceLUID, "id", "", "exact published datasource LUID")
			_ = c.MarkFlagRequired("id")
			c.Flags().StringVar(&in.Tag, "tag", "", "exact tag label")
			_ = c.MarkFlagRequired("tag")
			return func(ctx context.Context) (any, error) { return editor.ChangeTag(ctx, in, preview) }
		})
	}
}
