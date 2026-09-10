package capability

// BatchSelectors declares bounded repetitions of one action, not workflows.
// Empty selectors support files only; identifiers enable repeated scalar flags.
// Configuration, credential, installation and global-state commands are excluded.
func BatchSelectors() map[string]string {
	return map[string]string{
		"workbook.inspect": "id", "workbook.pull": "id", "workbook.publish": "id", "workbook.move": "id", "workbook.update": "id", "workbook.delete": "id",
		"datasource.inspect": "id", "datasource.schema": "id", "datasource.pull": "id", "datasource.publish": "id", "datasource.move": "id", "datasource.update": "id", "datasource.delete": "id",
		"flow.inspect": "id", "flow.pull": "id", "flow.publish": "id", "flow.move": "id", "flow.update": "id", "flow.delete": "id",
		"project.create": "", "project.inspect": "id", "project.update": "id", "project.move": "id", "project.delete": "id",
		"admin.user.create": "", "admin.user.inspect": "id", "admin.user.update": "id", "admin.user.delete": "id",
		"admin.group.create": "", "admin.group.inspect": "id", "admin.group.update": "id", "admin.group.delete": "id",
		"admin.group.member.add": "user-id", "admin.group.member.remove": "user-id",
		"admin.permission.inspect": "id", "admin.permission.create": "id", "admin.permission.delete": "id",
		"pulse.definition.create": "", "pulse.definition.inspect": "id", "pulse.definition.pull": "id", "pulse.definition.publish": "id", "pulse.definition.delete": "id",
		"pulse.metric.inspect": "id", "pulse.metric.fork": "id", "pulse.metric.follow": "id", "pulse.metric.unfollow": "id", "pulse.metric.followers": "id", "pulse.metric.delete": "id",
		"lineage.pull": "id",
	}
}
