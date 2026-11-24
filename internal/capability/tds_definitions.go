package capability

// The public surface includes help for unqualified workflows. Only a qualified
// direct operation can acquire a command binding and enter the executable set.
func datasourceEditDefinitions() []Definition {
	const live = "Requires an authorized disposable source, exact-name reads, permission and entitlement observations, mutation acknowledgement and independent readback on a supported Tableau installation."
	const saveNew = "Requires verified destination placement and create-only protection when another publisher claims the name during submission."
	const overwrite = "Requires a documented work-copy overwrite route, original identity, and a revision check covering the entire published definition."
	const tags = "Requires an authorized tag addition and removal on an exact published datasource, including empty membership and independently observed readback."
	const read = "Requires authorized exact and collection reads on an eligible REST 3.30 datasource, including named absence and incomplete records."
	var defs []Definition
	for _, item := range []struct {
		verb, outcome, upstream, selectors, gate string
		mutation, files                          bool
	}{
		{"checkout", "Allocate a work copy and preserve checkout provenance.", "Create Work Copy", "Exact source LUID or name and project; optional new audit and template paths", live, true, true},
		{"elements", "Inspect published TDS columns and descriptions.", "Read Data Source Elements", "Exact published LUID; optional exact field and table names", read, false, false},
		{"work-copy", "Read and export an exact work copy, compare an intent file, or recover an attempt.", "Read Work Copy Elements", "Exact work-copy LUID, changes file or authenticated attempt selection", read, false, true},
		{"set-field-description", "Set a field description on the work copy and verify its exact value.", "Add or Update Work Copy Elements", "Exact copy and field; nonempty description", live, true, false},
		{"remove-field-description", "Remove a field description from the work copy and verify absence.", "Delete Work Copy Elements", "Exact copy and field", live, true, false},
		{"set-table-description", "Set a table description on the work copy and verify its exact value.", "Add or Update Work Copy Elements", "Exact copy and table; nonempty description", live, true, false},
		{"remove-table-description", "Remove a table description from the work copy and verify absence.", "Delete Work Copy Elements", "Exact copy and table", live, true, false},
		{"set-column-caption", "Set one column caption in a typed changes file.", "Update Work Copy Elements", "Bound YAML target and exact field; caption", live, true, false},
		{"set-column-type", "Set one column type in a typed changes file.", "Update Work Copy Elements", "Bound YAML target and exact field; Tableau FieldType", live, true, false},
		{"set-column-role", "Set one column role in a typed changes file.", "Update Work Copy Elements", "Bound YAML target and exact field; Tableau FieldRole", live, true, false},
		{"delete-column", "Delete one named column in a typed changes file.", "Delete Work Copy Elements", "Bound YAML target and exact field", live, true, false},
		{"apply-changes", "Preview or apply ordered edits and preserve each observed result.", "Add, Update and Delete Work Copy Elements", "One bound changes file with up to 50 typed edits", live, true, false},
		{"save.overwrite", "Save the work copy over its verified original.", "Unverified overwrite contract", "Exact copy or changes file and verified source revision", overwrite, true, true},
		{"save.as-new", "Save the work copy to a new datasource and verify identity, placement and selected properties.", "Publish Work Copy", "Exact copy or changes file; new datasource name and optional description", saveNew, true, true},
		{"add-tag", "Add one published datasource tag and read back membership.", "Add Tags to Data Source", "Published datasource LUID and exact tag label", tags, true, false},
		{"remove-tag", "Remove one published datasource tag and read back membership.", "Delete Tag from Data Source", "Published datasource LUID and exact tag label", tags, true, false},
	} {
		id := "datasource." + item.verb
		d := Definition{
			ID:              id,
			Surface:         "tadx content datasource " + item.verb,
			Outcome:         item.outcome,
			Type:            OperationInspect,
			Disposition:     DispositionShip,
			Owner:           OwnerCLI,
			Selectors:       item.selectors,
			Availability:    "REST 3.30+, Tableau+ Cloud or Server Data Management; provider qualification pending",
			RemoteMutation:  item.mutation,
			SupportsPreview: item.mutation,
			LocalWrite:      item.files,
			SafetyGuard:     "Exact identities, operation qualification and independent scope checks",
			ArtifactEffect:  "None",
			Upstream:        item.upstream,
			Evidence:        "docs/evidence/tds-edit-contract.md",
			EvidenceLevel:   EvidenceContractVerified,
			Verification:    VerificationBlocked,
			Implementation:  ImplementationGated,
			Validation:      item.gate,
			Blocker:         BlockerProviderQualification,
		}
		if item.mutation {
			d.Type = OperationChange
			d.SafetyGuard = "Exact target, managed policy, saved site consent, fresh preflight and readback; independent provider activation gate"
		}
		if item.files {
			d.ArtifactEffect = "Private scoped recovery receipts or explicitly requested workspace exports according to operation"
		}
		switch item.verb {
		case "set-column-caption", "set-column-type", "set-column-role", "delete-column":
			d.Surface = "tadx content datasource apply-changes (YAML op " + item.verb + ")"
		case "save.overwrite":
			d.Surface = "tadx content datasource save overwrite"
			d.EvidenceLevel = EvidenceDocsOnly
			d.Implementation = ImplementationPlanned
		case "save.as-new":
			d.Surface = "tadx content datasource save as-new"
		case "add-tag", "remove-tag":
			d.Availability = "Tableau datasource REST tagging API 1.0+ with an authenticated TADX connection; provider qualification pending"
		}
		defs = append(defs, d)
	}
	return append(defs, Definition{
		ID:             "datasource.validate-changes",
		Surface:        "tadx content datasource validate-changes",
		Outcome:        "Validate one local changes file and list its ordered edit kinds.",
		Type:           OperationInspect,
		Disposition:    DispositionShip,
		Owner:          OwnerCLI,
		Selectors:      "Workspace and relative --from-changes path",
		Availability:   "Local",
		SafetyGuard:    "Strict typed YAML validation of a bounded local workspace file",
		ArtifactEffect: "None",
		Upstream:       "None",
		Evidence:       "docs/evidence/tds-edit-contract.md",
		EvidenceLevel:  EvidenceLocalContract,
		Verification:   VerificationReady,
		Implementation: ImplementationImplemented,
		Validation:     "Validates edit and target syntax. Remote eligibility requires live preflight.",
		CommandPath:    []string{"content", "datasource", "validate-changes"},
	})
}
