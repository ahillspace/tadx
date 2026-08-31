package publish_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/workbook/publish"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
)

type artifactReader struct{ artifact publish.Artifact }

func (r artifactReader) ReadWorkbook(context.Context, string) (publish.Artifact, error) {
	return r.artifact, nil
}

type resolver struct {
	project  publish.Project
	existing []publish.Workbook
}

func (r resolver) ResolveProject(context.Context, identity.Selector) (publish.Project, error) {
	return r.project, nil
}

func (r resolver) FindWorkbooks(context.Context, string, string) ([]publish.Workbook, error) {
	return r.existing, nil
}

type publisher struct {
	calls  int
	input  publish.PublishRequest
	result publish.Result
	err    error
}

type changingResolver struct {
	project  publish.Project
	results  [][]publish.Workbook
	findCall int
}

func (r *changingResolver) ResolveProject(context.Context, identity.Selector) (publish.Project, error) {
	return r.project, nil
}

func (r *changingResolver) FindWorkbooks(context.Context, string, string) ([]publish.Workbook, error) {
	result := r.results[r.findCall]
	r.findCall++
	return result, nil
}

func (p *publisher) Publish(_ context.Context, input publish.PublishRequest) (publish.Result, error) {
	p.calls++
	p.input = input
	return p.result, p.err
}

func TestPlanIsPreviewOnlyAndIncludesExplicitTarget(t *testing.T) {
	p := &publisher{}
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twbx", Name: "Finance"}},
		resolver{project: publish.Project{LUID: "project-1", Name: "Ops", Path: "Department/Ops"}}, p,
	)
	plan, err := action.Plan(context.Background(), publish.Input{
		ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing",
		ProjectSelector: identity.Selector{ProjectPath: "Department/Ops"}, AsJob: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 0 || plan.Mode != "preview" || plan.Target.ProjectLUID != "project-1" || !plan.AsJob {
		t.Fatalf("plan = %#v, publish calls = %d", plan, p.calls)
	}
}

func TestApplyExecutesOnlyPlanProducedByPlan(t *testing.T) {
	p := &publisher{result: publish.Result{Status: "succeeded", WorkbookLUID: "wb-new", JobID: "job-1"}}
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		resolver{project: publish.Project{LUID: "project-1", Name: "Ops", Path: "Ops"}}, p,
	)
	plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	result, err := action.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if p.calls != 1 || result.WorkbookLUID != "wb-new" || p.input.ProjectLUID != "project-1" {
		t.Fatalf("result = %#v, request = %#v", result, p.input)
	}
	if _, err := action.Apply(context.Background(), publish.Plan{}); err == nil {
		t.Fatal("Apply() accepted an unplanned mutation")
	}
}

func TestPlanRejectsCollisionWithoutExplicitOverwrite(t *testing.T) {
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		resolver{project: publish.Project{LUID: "project-1", Path: "Ops"}, existing: []publish.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1"}}}, &publisher{},
	)
	_, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}})
	if err == nil || !strings.Contains(err.Error(), "overwrite") {
		t.Fatalf("error = %v", err)
	}
}

func TestPlanRequiresExplicitWriteEnvironment(t *testing.T) {
	action := publish.New(artifactReader{}, resolver{}, &publisher{})
	_, err := action.Plan(context.Background(), publish.Input{ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "project-1"}})
	if err == nil || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("error = %v", err)
	}
}

func TestPlanAcceptsResolvedDefaultSite(t *testing.T) {
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		resolver{project: publish.Project{LUID: "project-1", Path: "Ops"}}, &publisher{},
	)
	plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", TargetResolved: true, ProjectSelector: identity.Selector{LUID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Site != "" {
		t.Fatalf("site = %q", plan.Target.Site)
	}
}

func TestApplyRejectsChangedOverwriteTarget(t *testing.T) {
	r := &changingResolver{
		project: publish.Project{LUID: "project-1", Path: "Ops"},
		results: [][]publish.Workbook{
			{{LUID: "wb-planned", Name: "Finance", ProjectLUID: "project-1"}},
			{{LUID: "wb-replacement", Name: "Finance", ProjectLUID: "project-1"}},
		},
	}
	p := &publisher{}
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		r, p,
	)
	plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = action.Apply(context.Background(), plan)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workbook.overwrite.target_changed" {
		t.Fatalf("error = %T %v", err, err)
	}
	if p.calls != 0 {
		t.Fatalf("publish calls = %d", p.calls)
	}
}

func TestPreviewGoldenOutput(t *testing.T) {
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twbx", Name: "Finance", Fingerprint: "sha256:abc"}},
		resolver{project: publish.Project{LUID: "project-1", Name: "Ops", Path: "Ops"}},
		&publisher{},
	)
	value, err := action.Execute(context.Background(), publish.Input{
		ArtifactPath:    `C:\workspace\Finance`,
		Environment:     "production",
		Site:            "marketing",
		ProjectSelector: identity.Selector{LUID: "project-1"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/preview.toon")
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}

func TestApplyReportsUnknownAsyncOutcomeWithoutSuggestingRetry(t *testing.T) {
	p := &publisher{result: publish.Result{Status: "unknown", JobID: "job-1", TableauRequestID: "poll-request"}, err: errors.New("poll forbidden")}
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		resolver{project: publish.Project{LUID: "project-1", Path: "Ops"}}, p,
	)
	plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}, AsJob: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = action.Apply(context.Background(), plan)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %v", err, err)
	}
	if structured.TableauJobID != "job-1" || structured.TableauRequestID != "poll-request" || !strings.Contains(strings.ToLower(structured.Summary), "outcome") || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("structured error = %#v", structured)
	}
}

func TestApplyReportsUnknownOutcomeWithoutClaimingAcceptance(t *testing.T) {
	p := &publisher{result: publish.Result{Status: "unknown", TableauRequestID: "publish-request"}, err: errors.New("decode publish response")}
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		resolver{project: publish.Project{LUID: "project-1", Path: "Ops"}}, p,
	)
	plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = action.Apply(context.Background(), plan)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %v", err, err)
	}
	if structured.ID != "workbook.publish.outcome_unknown" || structured.TableauRequestID != "publish-request" || !strings.Contains(structured.CorrectiveAction, "Inspect") {
		t.Fatalf("structured error = %#v", structured)
	}
	if strings.Contains(strings.ToLower(structured.Summary), "accepted") {
		t.Fatalf("summary claims acceptance: %q", structured.Summary)
	}
}
