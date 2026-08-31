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

type artifactReader struct {
	artifact publish.Artifact
	err      error
}

func (r artifactReader) ReadWorkbook(context.Context, string) (publish.Artifact, error) {
	return r.artifact, r.err
}

type resolver struct {
	project      publish.Project
	existing     []publish.Workbook
	projectErr   error
	collisionErr error
}

func (r resolver) ResolveProject(context.Context, identity.Selector) (publish.Project, error) {
	return r.project, r.projectErr
}

func (r resolver) FindWorkbooks(context.Context, string, string) ([]publish.Workbook, error) {
	return r.existing, r.collisionErr
}

type publisher struct {
	calls     int
	input     publish.PublishRequest
	result    publish.Result
	err       error
	onPrepare func()
}

type changingResolver struct {
	project  publish.Project
	results  [][]publish.Workbook
	findCall int
}

type retryableResolverError struct{}

func (retryableResolverError) Error() string            { return "Tableau unavailable" }
func (retryableResolverError) Retryable() bool          { return true }
func (retryableResolverError) CorrectiveAction() string { return "Retry after Tableau recovers." }

type revalidationResolver struct {
	project publish.Project
	calls   int
}

func (r *revalidationResolver) ResolveProject(context.Context, identity.Selector) (publish.Project, error) {
	return r.project, nil
}

func (r *revalidationResolver) FindWorkbooks(context.Context, string, string) ([]publish.Workbook, error) {
	r.calls++
	if r.calls == 1 {
		return []publish.Workbook{{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1"}}, nil
	}
	return nil, retryableResolverError{}
}

func (r *changingResolver) ResolveProject(context.Context, identity.Selector) (publish.Project, error) {
	return r.project, nil
}

func (r *changingResolver) FindWorkbooks(context.Context, string, string) ([]publish.Workbook, error) {
	result := r.results[r.findCall]
	r.findCall++
	return result, nil
}

func (p *publisher) Prepare(_ context.Context, input publish.PublishRequest) (publish.PreparedPublish, error) {
	p.input = input
	if p.onPrepare != nil {
		p.onPrepare()
	}
	return p, nil
}

func (p *publisher) Commit(context.Context) (publish.Result, error) {
	p.calls++
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

func TestPlanCompletesPlanningErrorAdvice(t *testing.T) {
	tests := []struct {
		name      string
		artifacts artifactReader
		resolver  resolver
	}{
		{name: "artifact", artifacts: artifactReader{err: errors.New("artifact invalid")}, resolver: resolver{}},
		{name: "project", artifacts: artifactReader{artifact: publish.Artifact{Path: "artifact", Filename: "Finance.twb", Name: "Finance"}}, resolver: resolver{projectErr: errors.New("project unavailable")}},
		{name: "collision", artifacts: artifactReader{artifact: publish.Artifact{Path: "artifact", Filename: "Finance.twb", Name: "Finance"}}, resolver: resolver{project: publish.Project{LUID: "project-1"}, collisionErr: errors.New("list unavailable")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := publish.New(test.artifacts, test.resolver, &publisher{}).Plan(context.Background(), publish.Input{ArtifactPath: "artifact", Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}})
			payload := errs.Structure(err).Error
			if payload.Retryable == nil || *payload.Retryable || payload.CorrectiveAction == "" || payload.Operation != "workbook.publish" {
				t.Fatalf("structured error = %#v", payload)
			}
		})
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
	if !errors.As(err, &structured) || structured.ID != "workbook.publish.target_changed" {
		t.Fatalf("error = %T %v", err, err)
	}
	if p.calls != 0 {
		t.Fatalf("publish calls = %d", p.calls)
	}
}

func TestApplyRevalidatesOverwriteTargetAfterPublishPreparation(t *testing.T) {
	r := &changingResolver{
		project: publish.Project{LUID: "project-1", Path: "Ops"},
		results: [][]publish.Workbook{
			{{LUID: "wb-planned", Name: "Finance", ProjectLUID: "project-1"}},
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
	if !errors.As(err, &structured) || structured.ID != "workbook.publish.target_changed" {
		t.Fatalf("error = %T %v", err, err)
	}
	if p.calls != 0 || r.findCall != 3 {
		t.Fatalf("commit calls = %d, collision reads = %d", p.calls, r.findCall)
	}
}

func TestApplyPreservesRetryAdviceForOverwriteRevalidation(t *testing.T) {
	r := &revalidationResolver{project: publish.Project{LUID: "project-1", Path: "Ops"}}
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twb", Name: "Finance"}},
		r, &publisher{},
	)
	plan, err := action.Plan(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}, Overwrite: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = action.Apply(context.Background(), plan)
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestPlanDefaultsTargetToArtifactSource(t *testing.T) {
	action := publish.New(
		artifactReader{artifact: publish.Artifact{
			Path: `C:\workspace\Finance`, Filename: "Finance.twbx", Name: "Finance", TableauID: "wb-src",
			SourceEnvironment: "production", SourceSite: "marketing",
			SourceProjectName: "Department/Ops", SourceProjectID: "project-1",
		}},
		resolver{
			project:  publish.Project{LUID: "project-1", Name: "Ops", Path: "Department/Ops"},
			existing: []publish.Workbook{{LUID: "wb-src", Name: "Finance", ProjectLUID: "project-1"}},
		},
		&publisher{},
	)
	plan, err := action.Plan(context.Background(), publish.Input{
		ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing",
		TargetResolved: true, SourceDefaulted: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Origin != "artifact-source" || plan.Target.ExistingLUID != "wb-src" || !plan.Overwrite || plan.WorkbookName != "Finance" || plan.Target.ProjectLUID != "project-1" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanFailsWhenArtifactSourceChanged(t *testing.T) {
	tests := []struct {
		name     string
		existing []publish.Workbook
	}{
		{name: "deleted-or-moved", existing: nil},
		{name: "name-now-points-elsewhere", existing: []publish.Workbook{{LUID: "wb-other", Name: "Finance", ProjectLUID: "project-1"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action := publish.New(
				artifactReader{artifact: publish.Artifact{Path: "artifact", Filename: "Finance.twbx", Name: "Finance", TableauID: "wb-src", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"}},
				resolver{project: publish.Project{LUID: "project-1", Path: "Department/Ops"}, existing: test.existing},
				&publisher{},
			)
			_, err := action.Plan(context.Background(), publish.Input{ArtifactPath: "artifact", Environment: "production", Site: "marketing", TargetResolved: true, SourceDefaulted: true})
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.ID != "workbook.publish.source_changed" {
				t.Fatalf("error = %T %v", err, err)
			}
			if structured.Retryable == nil || *structured.Retryable {
				t.Fatalf("source_changed must be non-retryable: %#v", structured)
			}
		})
	}
}

func TestSourcePreviewGoldenOutput(t *testing.T) {
	action := publish.New(
		artifactReader{artifact: publish.Artifact{Path: `C:\workspace\Finance`, Filename: "Finance.twbx", Name: "Finance", TableauID: "wb-src", Fingerprint: "sha256:abc", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"}},
		resolver{project: publish.Project{LUID: "project-1", Name: "Ops", Path: "Department/Ops"}, existing: []publish.Workbook{{LUID: "wb-src", Name: "Finance", ProjectLUID: "project-1"}}},
		&publisher{},
	)
	value, err := action.Execute(context.Background(), publish.Input{ArtifactPath: `C:\workspace\Finance`, Environment: "production", Site: "marketing", TargetResolved: true, SourceDefaulted: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/preview_source.toon")
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
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
