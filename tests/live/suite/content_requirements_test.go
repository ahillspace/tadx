package suite

import (
	"slices"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

func TestContentFixtureCapabilitiesMatchActualRecoveryCalls(t *testing.T) {
	if caps := ContentFixtureCapabilities("live.workbook.schema"); len(caps) != 0 {
		t.Fatalf("bound schema read invents fixture mutations: %v", caps)
	}
	for _, id := range []string{"live.workbook.pull", "live.datasource.detached-publish", "live.flow.detached-pull"} {
		if !slices.Contains(ContentFixtureCapabilities(id), "job.inspect") {
			t.Errorf("missing detached cleanup recovery capability for %s", id)
		}
	}
	if caps := ContentFixtureCapabilities("live.flow.detached-publish"); slices.Contains(caps, "job.wait") {
		t.Fatalf("synchronous flow invents Tableau job wait: %v", caps)
	}
	if !slices.Contains(ContentFixtureCapabilities("live.project.move"), "workbook.publish") {
		t.Error("project move omits descendant workbook fixture publication")
	}
	if !slices.Contains(SharedFixtureCapabilities("live.cache.permission-generation"), "admin.permission.create") {
		t.Error("permission generation omits explicit fixture permission mutation")
	}
	if !slices.Contains(SharedFixtureCapabilities("live.contract.permission-refusal"), "workbook.update") {
		t.Error("permission refusal omits durable restoration update")
	}
}

func TestContentRequirementsCoverOnlyRegisteredScenarios(t *testing.T) {
	cases := ContentCases()
	requirements := ContentBindingRequirements()
	if len(cases) != 53 || len(requirements) != len(cases) {
		t.Fatalf("case/requirement counts=%d/%d", len(cases), len(requirements))
	}
	for id := range cases {
		if _, ok := requirements[id]; !ok {
			t.Errorf("missing prerequisites for %s", id)
		}
	}
	for id := range requirements {
		if _, ok := cases[id]; !ok {
			t.Errorf("requirements reference missing scenario %s", id)
		}
	}
	if err := ValidateContentRequirements(harness.Profile{}, []string{"live.project.create"}); err != nil {
		t.Fatalf("core project fixture requires external IDs: %v", err)
	}
	if err := ValidateContentRequirements(harness.Profile{}, []string{"live.workbook.update"}); harness.ErrorStatus(err) != harness.BlockedStatus {
		t.Fatalf("missing fixture pack must block before setup writes: %v", err)
	}
}

func TestContentRequirementsRejectMalformedFreshJobsAndDependencies(t *testing.T) {
	for _, binding := range []harness.Binding{
		{Params: map[string]string{"submit_method": "GET", "submit_path": "/api/3.29/submit"}},
		{Params: map[string]string{"submit_method": "POST", "submit_path": "https://other.invalid/submit"}},
		{Params: map[string]string{"submit_method": "POST", "submit_path": "/api/3.29/submit", "submit_body": "{"}},
	} {
		if err := ValidateContentBinding("job-wait", binding); err == nil {
			t.Errorf("unsafe or invalid submission binding accepted: %#v", binding)
		}
	}
	malformed := harness.Binding{Expected: map[string]any{"direct_ids": []any{map[string]any{}, map[string]any{}}, "grandparent_id": "g", "lineage": map[string]any{"edges": []any{}}}}
	if err := ValidateContentBinding("workbook-dependencies", malformed); err == nil {
		t.Error("invalid dependency LUID values accepted")
	}
}
