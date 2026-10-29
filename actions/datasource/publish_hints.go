package datasource

import "github.com/ahillspace/tadx/internal/commandhint"

func publishPublishInspectionHint(plan PublishPlan, result PublishResult) string {
	id := result.DatasourceLUID
	if id != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "datasource", "inspect", "--id", id)
	}
	if plan.Target.ExistingLUID != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "datasource", "inspect", "--id", plan.Target.ExistingLUID)
	}
	if plan.DatasourceName != "" && plan.Target.ProjectPath != "" {
		return commandhint.Environment(plan.Target.Environment, "content", "datasource", "inspect", "--name", plan.DatasourceName, "--project", plan.Target.ProjectPath)
	}
	return ""
}
