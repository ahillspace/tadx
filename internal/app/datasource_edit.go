package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	ds "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/capability"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/operationrun"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

func (c *remoteContentCommands) tdsBlocker(id string) string {
	defs := c.tdsDefinitions
	if defs == nil {
		defs = capability.All()
	}
	for _, d := range defs {
		if d.ID == id {
			if d.Verification == capability.VerificationReady {
				return ""
			}
			return d.Validation
		}
	}
	return "This operation has no qualified provider contract."
}

func (c *remoteContentCommands) editFiles(ctx context.Context, env string, f contentcli.EditFiles) (root string, doc ds.EditDocument, err error) {
	seen := make(map[string]bool, len(f.TemplateFields))
	for _, name := range f.TemplateFields {
		if strings.TrimSpace(name) == "" || seen[name] || len(f.TemplateFields) > 50 {
			return "", doc, errs.New(errs.KindUsage, "Choose at most 50 distinct nonblank --template-field names.")
		}
		seen[name] = true
	}
	if len(f.TemplateFields) > 0 && f.ChangesFile == "" {
		return "", doc, errs.New(errs.KindUsage, "--template-field requires --changes-file")
	}
	if f.AuditFile == "" && f.ChangesFile == "" && f.FromChanges == "" {
		if f.Workspace != "" {
			return "", doc, errs.New(errs.KindUsage, "--workspace requires an input or output file")
		}
		return "", doc, nil
	}
	ws, err := (&workspaceRuntime{runtime: c.runtime}).resolveForEnvironment(ctx, f.Workspace, env)
	if err != nil {
		return "", doc, err
	}
	if f.FromChanges != "" {
		doc, err = ds.LoadEditDocument(ws.Root, f.FromChanges)
		if err != nil {
			return "", doc, err
		}
	}
	if f.AuditFile != "" || f.ChangesFile != "" {
		if err = ds.ValidateEditExports(ws.Root, f.AuditFile, f.ChangesFile); err != nil {
			return "", doc, err
		}
	}
	return ws.Root, doc, nil
}
func (c *remoteContentCommands) ValidateChanges(ctx context.Context, in contentcli.EditFiles) (any, error) {
	if strings.TrimSpace(in.FromChanges) == "" {
		return nil, errs.New(errs.KindUsage, "--from-changes requires a workspace-relative input file.")
	}
	_, doc, err := c.editFiles(ctx, "", in)
	if err != nil {
		return nil, err
	}
	operations := make([]string, len(doc.Changes))
	for i, ch := range doc.Changes {
		operations[i] = ch.Op
	}
	return struct {
		Status     string        `json:"status"`
		Target     ds.EditTarget `json:"target"`
		Operations []string      `json:"operations"`
	}{"valid", doc.Target, operations}, nil
}

type tdsConnection struct {
	remoteConnection
	scope   value.TDSScope
	journal operationrun.TDSJournal
}

func (c *remoteContentCommands) tdsConnect(ctx context.Context, op, env string, write bool) (tdsConnection, error) {
	conn, err := c.connect(ctx, env, write)
	if err != nil {
		return tdsConnection{}, remoteSetupError(op, env, "", conn.environment, err)
	}
	server, err := config.CanonicalMutationServer(conn.environment.URL)
	if err != nil {
		return tdsConnection{}, err
	}
	if err := checkTDSText(conn.tds.TDSRedactor(), server, conn.siteLUID, conn.callerLUID, conn.environment.Alias, conn.environment.SiteContentURL); err != nil {
		return tdsConnection{}, err
	}
	journal, err := operationrun.DefaultTDSJournal()
	if err != nil {
		return tdsConnection{}, err
	}
	return tdsConnection{conn, value.TDSScope{Server: server, SiteLUID: conn.siteLUID, CallerLUID: conn.callerLUID}, journal}, nil
}
func (c tdsConnection) editTarget(ctx context.Context, id string, expect ds.EditTarget) (ds.EditTarget, error) {
	origin, err := c.journal.Origin(ctx, c.scope, id)
	if err != nil {
		return ds.EditTarget{}, err
	}
	t := ds.EditTarget{Server: c.scope.Server, SiteLUID: c.siteLUID, WorkCopyLUID: id, DatasourceLUID: origin.SourceLUID}
	if expect.WorkCopyLUID != "" {
		err = expect.Match(t)
	}
	return t, err
}
func (c *remoteContentCommands) attemptOutput() io.Writer {
	if c.runtime.progressWriter != nil {
		return c.runtime.progressWriter
	}
	return os.Stderr
}

// An ordered edit or a slow checkout can outlive its initial consent check.
// Bind every dispatch to the environment selected for this command.
func (c *remoteContentCommands) tdsWriteContext(ctx context.Context, operation string, env config.Environment, preview bool) context.Context {
	if preview {
		return ctx
	}
	return tableau.WithSubmissionCheck(ctx, func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		cfg, err := config.Load(c.runtime.configPath)
		if err == nil {
			var enabled value.MutationSetting
			enabled, err = siteMutationSetting(cfg, env)
			if err == nil && enabled.Enabled {
				return nil
			}
		}
		return &errs.Error{ID: "mutation.disabled", Operation: operation, Environment: env.Alias, Site: env.SiteContentURL, Kind: errs.KindOperation, Summary: "Site consent could not be confirmed at submission.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: new(false), Cause: err, CorrectiveAction: commandhint.Environment(env.Alias, "mutation", "status")}
	})
}

func (c *remoteContentCommands) CheckoutDatasource(ctx context.Context, in contentcli.CheckoutOptions) (any, error) {
	input := ds.CheckoutInput{Environment: in.Environment, Selector: identity.Selector{LUID: identity.LUID(in.ID), Name: in.Name, ProjectLUID: identity.LUID(in.ProjectID), ProjectPath: in.Project}, AcknowledgeUnknown: in.Acknowledge, AttemptOutput: c.attemptOutput()}
	if err := ds.ValidateCheckoutInput(input); err != nil {
		return nil, err
	}
	if in.Preview && (in.AuditFile != "" || in.ChangesFile != "") {
		return nil, errs.New(errs.KindUsage, "Checkout exports require an allocated copy. Remove output flags for preview.")
	}
	root, _, err := c.editFiles(ctx, in.Environment, in.EditFiles)
	if err != nil {
		return nil, err
	}
	conn, err := c.tdsConnect(ctx, "datasource.checkout", in.Environment, true)
	if err != nil {
		return nil, err
	}
	if err := checkTDSEditInputs(conn.tds.TDSRedactor(), in.EditFiles, ds.EditDocument{}, append([]string{in.Environment, in.ID, in.Name, in.Project, in.ProjectID}, in.Acknowledge...)...); err != nil {
		return nil, err
	}
	input.Eligibility = conn.tds.TDSPrerequisites("datasource.checkout")
	input.Scope, input.Environment = conn.scope, conn.environment.Alias
	var resolver ds.Resolver = resourcedatasource.TDSResolver{Adapter: conn.datasources}
	if revisions, ok := c.saveProvider.(ds.SourceRevisionReader); ok {
		resolver = struct {
			ds.Resolver
			ds.SourceRevisionReader
		}{resolver, revisions}
	}
	ctx = c.tdsWriteContext(ctx, "datasource.checkout", conn.environment, in.Preview)
	out, err := ds.Checkout(ctx, resolver, conn.tds, conn.journal, input, in.Preview)
	if err != nil || root == "" || out.WorkCopyLUID == "" {
		return out, err
	}
	target := ds.EditTarget{Server: conn.scope.Server, SiteLUID: conn.siteLUID, WorkCopyLUID: out.WorkCopyLUID, DatasourceLUID: out.SourceLUID}
	inspect, err := c.inspectAndExport(ctx, conn, root, target, out.Checkout, in.EditFiles, nil, nil)
	if err != nil {
		err = &errs.Error{ID: "datasource.checkout.exports", Kind: errs.KindOperation, Operation: out.Operation, Resource: out.WorkCopyLUID, Summary: "Checkout succeeded. Requested exports or observations are incomplete.", Cause: err, Phase: errs.PhasePersistence, Outcome: errs.OutcomeConfirmed, Retryable: new(false)}
	}
	out.Files = inspect.Files
	out.ObservationsComplete = &inspect.Complete
	out.MetadataWarning = inspect.MetadataWarning
	out.ExportHelp = inspect.Help
	return out, err
}

func (c *remoteContentCommands) inspectAndExport(ctx context.Context, conn tdsConnection, root string, target ds.EditTarget, checkout *value.TDSCheckoutContext, f contentcli.EditFiles, fields, tables []string) (ds.CopyInspection, error) {
	redact := conn.tds.TDSRedactor()
	inspect, readErr := ds.InspectTDSElements(ctx, conn.tds, "workcopy", target, conn.environment.Alias, fields, tables)
	inspect.Checkout = checkout
	if f.AuditFile == "" && f.ChangesFile == "" {
		return inspect, readErr
	}
	if f.AuditFile != "" {
		inspect.MetadataWarning = "Published source identity is unknown."
		if target.DatasourceLUID != "" {
			if err := c.runtime.checkManagedCapability("datasource.schema"); err != nil {
				inspect.MetadataWarning = "Published datasource Metadata API enrichment is denied by managed policy."
			} else {
				includeLineage := c.runtime.checkManagedCapability("lineage.pull") == nil
				metadata, err := conn.metadataAssets.DatasourceAuditFields(ctx, target.DatasourceLUID, includeLineage)
				switch {
				case metadata.Identity.MetadataID == "" || metadata.Identity.LUID != target.DatasourceLUID:
					inspect.MetadataWarning = "Published datasource Metadata API enrichment was unavailable."
				case err != nil || !metadata.Complete:
					inspect.SourceMetadata = &metadata
					inspect.MetadataWarning = "The Metadata API returned partial published datasource observations."
				default:
					inspect.SourceMetadata = &metadata
					inspect.MetadataWarning = ""
				}
				if inspect.SourceMetadata != nil && !includeLineage {
					inspect.MetadataWarning = strings.TrimSpace(inspect.MetadataWarning + " Managed policy denies lineage.pull. Calculation dependencies and downstream sheets were omitted.")
				}
			}
		}
	}
	// Named reads can reach an explicitly requested example outside the audit's
	// bounded collection. Preserve the original audit scope.
	if missing := inspect.UnobservedTemplateFields(f.TemplateFields); len(missing) > 0 {
		named, e := ds.InspectTDSElements(ctx, conn.tds, "workcopy", target, conn.environment.Alias, missing, nil)
		inspect.Observations = append(inspect.Observations, named.Observations...)
		inspect.Complete = inspect.Complete && named.Complete
		readErr = errors.Join(readErr, e)
	}
	if f.ChangesFile != "" {
		inspect.Help = append(inspect.Help, commandhint.Target(conn.environment.Alias, f.Workspace, "content", "datasource", "apply-changes", "--from-changes", f.ChangesFile, "--preview"))
	}
	files, err := ds.ExportEditCopy(root, f.AuditFile, f.ChangesFile, inspect, f.TemplateFields, redact)
	if err != nil {
		args := []string{"content", "datasource", "work-copy", "--work-copy", target.WorkCopyLUID}
		if f.Workspace != "" {
			args = append(args, "--workspace", f.Workspace)
		}
		if f.AuditFile != "" && files.AuditFile == "" {
			args = append(args, "--audit-file", f.AuditFile)
		}
		if f.ChangesFile != "" && files.ChangesFile == "" {
			args = append(args, "--changes-file", f.ChangesFile)
			for _, field := range f.TemplateFields {
				args = append(args, "--template-field", field)
			}
		}
		inspect.Help = []string{commandhint.Environment(conn.environment.Alias, args...)}
	}
	inspect.Files = &files
	return inspect, errors.Join(readErr, err)
}

func (c *remoteContentCommands) InspectElements(ctx context.Context, in contentcli.InspectionOptions) (any, error) {
	attempts := in.ListAttempts || in.Attempt != ""
	if in.Verify && in.Attempt == "" || in.Attempt != "" && (in.ListAttempts || in.SourceID != "" || in.WorkCopy != "") || attempts && (len(in.Fields)+len(in.Tables) > 0 || in.EditFiles.Workspace != "" || in.AuditFile != "" || in.ChangesFile != "" || in.FromChanges != "" || len(in.TemplateFields) > 0) {
		return nil, errs.New(errs.KindUsage, "Use --attempt with optional --verify, or --attempts with an optional source or copy filter.")
	}
	if in.FromChanges != "" && (in.WorkCopy != "" || in.SourceID != "" || len(in.Fields)+len(in.Tables) > 0 || in.AuditFile != "" || in.ChangesFile != "") {
		return nil, errs.New(errs.KindUsage, "A review file supplies its own copy and selection.")
	}
	if in.SourceID != "" && !value.ValidTDSLUID(in.SourceID) || in.WorkCopy != "" && !value.ValidTDSLUID(in.WorkCopy) {
		return nil, errs.New(errs.KindUsage, "Provide an exact datasource or work-copy UUID.")
	}
	root, doc, err := c.editFiles(ctx, in.Environment, in.EditFiles)
	if err != nil {
		return nil, err
	}
	if in.FromChanges != "" {
		in.WorkCopy = doc.Target.WorkCopyLUID
	}
	if !attempts && in.SourceID == "" && in.WorkCopy == "" {
		return nil, errs.New(errs.KindUsage, "Select a work copy, changes file or recovery attempt.")
	}
	op := "datasource.work-copy"
	if !attempts && in.SourceID != "" {
		op = "datasource.elements"
	}
	conn, err := c.tdsConnect(ctx, op, in.Environment, false)
	if err != nil {
		return nil, err
	}
	args := []string{in.Environment, in.SourceID, in.WorkCopy, in.Attempt}
	args = append(args, in.Fields...)
	args = append(args, in.Tables...)
	if err := checkTDSEditInputs(conn.tds.TDSRedactor(), in.EditFiles, doc, args...); err != nil {
		return nil, err
	}
	if attempts {
		records, more, err := conn.journal.Attempts(ctx, conn.scope, in.Attempt, in.SourceID, in.WorkCopy)
		if err != nil {
			return nil, err
		}
		for _, a := range records {
			if err = ds.CheckRetainedAttempt(a, conn.tds.TDSRedactor()); err != nil {
				return nil, err
			}
		}
		if in.Attempt != "" && len(records) != 1 {
			return nil, errs.New(errs.KindOperation, "No matching attempt in the authenticated scope.")
		}
		if in.Verify {
			return ds.VerifySaveAttempt(ctx, resourcedatasource.TDSResolver{Adapter: conn.datasources}, conn.tds, records[0])
		}
		return ds.DescribeTDSAttempts(conn.environment.Alias, records, more), nil
	}
	if in.SourceID != "" {
		return ds.InspectTDSElements(ctx, conn.tds, "datasource", ds.EditTarget{Server: conn.scope.Server, SiteLUID: conn.siteLUID, DatasourceLUID: in.SourceID}, conn.environment.Alias, in.Fields, in.Tables)
	}
	origin, err := conn.journal.Origin(ctx, conn.scope, in.WorkCopy)
	if err != nil {
		return nil, err
	}
	if err := ds.CheckRetainedAttempt(origin, conn.tds.TDSRedactor()); err != nil {
		return nil, err
	}
	target := ds.EditTarget{Server: conn.scope.Server, SiteLUID: conn.siteLUID, WorkCopyLUID: in.WorkCopy, DatasourceLUID: origin.SourceLUID}
	if in.FromChanges != "" {
		if err = doc.Target.Match(target); err != nil {
			return nil, err
		}
		return ds.ReviewEdits(ctx, conn.tds, target, conn.environment.Alias, doc.Changes)
	}

	return c.inspectAndExport(ctx, conn, root, target, origin.Checkout, in.EditFiles, in.Fields, in.Tables)
}

func (c *remoteContentCommands) ApplyEdits(ctx context.Context, in contentcli.ApplyOptions) (result any, resultErr error) {
	_, doc, err := c.editFiles(ctx, in.Environment, in.EditFiles)
	if err != nil {
		return nil, err
	}
	op := "datasource.apply-changes"
	if in.FromChanges == "" {
		doc.Changes = []ds.EditChange{in.Change}
		op = in.Change.CapabilityID()
		if err = ds.ValidateEditChanges(doc.Changes); err != nil {
			return nil, err
		}
	} else {
		in.WorkCopy = doc.Target.WorkCopyLUID
		for _, ch := range doc.Changes {
			if err = c.runtime.checkManagedCapability(ch.CapabilityID()); err != nil {
				return nil, err
			}
			if reason := c.tdsBlocker(ch.CapabilityID()); reason != "" {
				return nil, &errs.Error{ID: "datasource.evidence_required", Kind: errs.KindOperation, Operation: ch.CapabilityID(), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Summary: reason}
			}
		}
	}
	if !value.ValidTDSLUID(in.WorkCopy) {
		return nil, errs.New(errs.KindUsage, "An exact work-copy UUID is required.")
	}
	conn, err := c.tdsConnect(ctx, op, in.Environment, true)
	if err != nil {
		return nil, err
	}
	if err := checkTDSEditInputs(conn.tds.TDSRedactor(), in.EditFiles, doc, in.WorkCopy, in.ExpectPreview); err != nil {
		return nil, err
	}
	target, err := conn.editTarget(ctx, in.WorkCopy, doc.Target)
	if err != nil {
		return nil, err
	}
	if !in.Preview {
		release, err := conn.journal.LockCopy(ctx, conn.scope, in.WorkCopy)
		if err != nil {
			return nil, err
		}
		defer func() { resultErr = errors.Join(resultErr, release()) }()
	}
	ctx = c.tdsWriteContext(ctx, op, conn.environment, in.Preview)
	out, err := ds.Edit(ctx, conn.tds, target, conn.environment.Alias, doc.Changes, in.Preview, in.ExpectPreview)
	for i := range out.Items {
		row := &out.Items[i]
		if row.Eligibility.Operation == "" {
			operation := "datasource." + row.Op
			switch row.Action {
			case "add", "update", "delete":
				operation = "datasource.tds." + row.Action
			}
			row.Eligibility = conn.tds.TDSPrerequisites(operation)
		}
	}
	return out, err
}

func (c *remoteContentCommands) SaveCopy(ctx context.Context, in contentcli.SaveOptions) (any, error) {
	_, doc, err := c.editFiles(ctx, in.Environment, in.EditFiles)
	if err != nil {
		return nil, err
	}
	if in.FromChanges != "" {
		in.WorkCopy = doc.Target.WorkCopyLUID
	}
	input := ds.SaveWorkCopyInput{Environment: in.Environment, Mode: in.Mode, WorkCopyLUID: in.WorkCopy, Name: in.Name, Description: in.Description, Fields: in.Fields, Changes: doc.Changes, FromChanges: in.FromChanges != "", AcknowledgeUnknown: in.Acknowledge, ExpectPreview: in.ExpectPreview, AttemptOutput: c.attemptOutput()}
	if err = ds.ValidateSaveWorkCopyInput(input); err != nil {
		return nil, err
	}
	conn, err := c.tdsConnect(ctx, "datasource.save."+in.Mode, in.Environment, true)
	if err != nil {
		return nil, err
	}
	redact := conn.tds.TDSRedactor()
	fields := append([]string{in.WorkCopy, in.Name, in.ExpectPreview}, in.Fields...)
	fields = append(fields, in.Acknowledge...)
	if in.Description != nil {
		fields = append(fields, *in.Description)
	}
	if err := checkTDSEditInputs(redact, in.EditFiles, doc, fields...); err != nil {
		return nil, err
	}
	if in.Mode == "as-new" {
		input.Eligibility = conn.tds.TDSPrerequisites("datasource.save.as-new")
	}
	input.Redact = redact
	if in.FromChanges != "" {
		if _, err = conn.editTarget(ctx, in.WorkCopy, doc.Target); err != nil {
			return nil, err
		}
	}
	input.Scope, input.Environment = conn.scope, conn.environment.Alias
	ctx = c.tdsWriteContext(ctx, "datasource.save."+in.Mode, conn.environment, in.Preview)
	return ds.SaveWorkCopy(ctx, c.saveProvider, conn.journal, input, in.Preview)
}
func (c *remoteContentCommands) ChangeTag(ctx context.Context, in ds.TagInput, preview bool) (ds.TagOutput, error) {
	if err := in.Validate(); err != nil {
		return ds.TagOutput{}, err
	}
	conn, err := c.connect(ctx, in.Environment, true)
	if err != nil {
		return ds.TagOutput{}, err
	}
	if err := checkTDSText(conn.tds.TDSRedactor(), conn.environment.Alias, in.DatasourceLUID, in.Tag); err != nil {
		return ds.TagOutput{}, err
	}
	in.Environment = conn.environment.Alias
	op := "datasource.remove-tag"
	if in.Add {
		op = "datasource.add-tag"
	}
	ctx = c.tdsWriteContext(ctx, op, conn.environment, preview)
	return ds.ChangeDatasourceTag(ctx, conn.tds, in, preview)
}
