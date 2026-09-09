package inspect

import (
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

// ValidateInput checks local permission filters without a remote session.
func ValidateInput(in Input) error {
	validKind := in.ResourceKind == "workbook" || in.ResourceKind == "datasource" || in.ResourceKind == "flow" || in.ResourceKind == "project"
	validDefault := in.DefaultFor == "" || in.ResourceKind == "project" && (in.DefaultFor == "workbooks" || in.DefaultFor == "datasources" || in.DefaultFor == "flows")
	validPrincipal := in.PrincipalType == "" || in.PrincipalType == "user" || in.PrincipalType == "group"
	if !validKind || strings.TrimSpace(in.ResourceLUID) == "" || !validDefault || !validPrincipal {
		return &errs.Error{ID: "admin.permission.inspect.usage", Kind: errs.KindUsage, Operation: "admin.permission.inspect", Summary: "Invalid permission inspection selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Use --kind workbook, datasource, flow, or project with an exact --id; --default-for applies only to projects and --principal-type is user or group."}
	}
	return nil
}
