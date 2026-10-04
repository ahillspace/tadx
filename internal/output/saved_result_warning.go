package output

type savedResultWarning struct {
	Code                 string `json:"code"`
	Summary              string `json:"summary"`
	LastPotentiallyStale bool   `json:"last_potentially_stale"`
	CorrectiveAction     string `json:"corrective_action"`
}

// LastResultWarning reports failed local persistence without changing the outcome.
func LastResultWarning() any {
	return struct {
		Warning savedResultWarning `json:"warning"`
	}{Warning: savedResultWarning{
		Code: "last_result_save_failed", Summary: "The current result could not be saved; the operation outcome is unchanged.",
		LastPotentiallyStale: true,
		CorrectiveAction:     "Retain this output. The previous last result can be stale; do not repeat a mutation to recover its receipt.",
	}}
}
