package app

import (
	ds "github.com/ahillspace/tadx/actions/datasource"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/ahillspace/tadx/internal/errs"
)

// Reject a credential-bearing selector or intended value before it can enter
// a request, an edit's partial output, a comparison scope or a recovery hint.
// Substituting a marker would alter the user's requested Tableau change.
func checkTDSText(redact func(string) string, inputs ...string) error {
	for _, s := range inputs {
		if s != "" && redact(s) != s {
			return &errs.Error{ID: "datasource.tds.protected_input", Kind: errs.KindUsage, Summary: "A datasource argument or changes-file value contains protected credential material.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Retryable: new(false)}
		}
	}
	return nil
}

func checkTDSEditInputs(redact func(string) string, f contentcli.EditFiles, doc ds.EditDocument, inputs ...string) error {
	if err := checkTDSText(redact, inputs...); err != nil {
		return err
	}
	if err := checkTDSText(redact, f.Workspace, f.FromChanges, f.AuditFile, f.ChangesFile, doc.Target.Server, doc.Target.SiteLUID, doc.Target.WorkCopyLUID, doc.Target.DatasourceLUID); err != nil {
		return err
	}
	if err := checkTDSText(redact, f.TemplateFields...); err != nil {
		return err
	}
	for _, c := range doc.Changes {
		if err := checkTDSText(redact, c.Field, c.Table, c.Description, c.Caption, c.Type, c.Role); err != nil {
			return err
		}
	}
	return nil
}
