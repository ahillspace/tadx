package publish

import "github.com/ahillspace/tadx/internal/commandhint"

func publishInspectionHint(plan Plan, result Result) string {
	id := result.WorkbookLUID
	if id == "" {
		id = plan.Target.ExistingLUID
	}
	if id != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "workbook", "inspect", "--id", id)
	}
	if plan.WorkbookName != "" && plan.Target.ProjectPath != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "workbook", "inspect", "--name", plan.WorkbookName, "--project", plan.Target.ProjectPath)
	}
	return ""
}
