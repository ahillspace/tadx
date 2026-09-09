package publish

import "github.com/ahillspace/tadx/internal/commandhint"

func publishInspectionHint(plan Plan, result Result) string {
	id := result.DatasourceLUID
	if id == "" {
		id = plan.Target.ExistingLUID
	}
	if id != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "datasource", "inspect", "--id", id)
	}
	if plan.DatasourceName != "" && plan.Target.ProjectPath != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "datasource", "inspect", "--name", plan.DatasourceName, "--project", plan.Target.ProjectPath)
	}
	return ""
}
