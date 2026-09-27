package value

// AgentGuidanceSkill is one home-relative native package observation.
type AgentGuidanceSkill struct {
	Target                             string
	Name, Status, Path, SHA256, Backup string
	Files                              int
}

// AgentGuidanceResult retains completed targets and recoverable backups.
type AgentGuidanceResult struct {
	Targets  []string
	Status   string
	Skills   []AgentGuidanceSkill
	Warnings []string
}
