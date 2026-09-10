package publish_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
)

func TestDatasourcePublishOutputGoldens(t *testing.T) {
	value := datasourcepublish.Output{Plan: datasourcepublish.Plan{Mode: "execute", PublishMode: datasourcepublish.ModeOverwrite, Operation: "datasource.publish", ArtifactPath: "artifacts/datasource/Sales", ArtifactFingerprint: "sha256:secret-diagnostic", Filename: "Sales.tds", DatasourceName: "Sales", CompositionStatus: "composed", ParentDataSourceURLs: []string{"parent-a", "parent-b"}, Target: datasourcepublish.Target{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1", ProjectPath: "Analytics", ExistingLUID: "ds-1"}, Substeps: []string{"resolve exact destination", "poll asynchronous job"}, AsJob: true}, Result: &datasourcepublish.Result{Status: "succeeded", DatasourceLUID: "ds-1", DatasourceName: "Sales", ProjectLUID: "project-1", JobID: "job-1", TableauRequestID: "request-1"}, Help: []string{"tadx content datasource inspect --id ds-1"}}
	assertDatasourcePublishGolden(t, "compact.toon", value, false)
	assertDatasourcePublishGolden(t, "full.toon", value, true)
	var compact bytes.Buffer
	if err := output.Render(&compact, value); err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{"artifact_fingerprint", "parent_datasource_urls", "substeps", "tableau_request_id", "composition_status", "filename"} {
		if strings.Contains(compact.String(), omitted) {
			t.Fatalf("compact output exposed %q:\n%s", omitted, compact.String())
		}
	}
	if !strings.Contains(compact.String(), "details: \"--full\"\nhelp[1]:") {
		t.Fatalf("details must immediately precede help:\n%s", compact.String())
	}
}

func assertDatasourcePublishGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(actual.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, actual.Bytes())
	}
}

type publishArtifactReader struct {
	artifact datasourcepublish.Artifact
	calls    int
}

func (r *publishArtifactReader) ReadDatasource(context.Context, string) (datasourcepublish.Artifact, error) {
	r.calls++
	return r.artifact, nil
}

type publishResolver struct {
	project         datasourcepublish.Project
	collisions      []datasourcepublish.Datasource
	completion      datasourcepublish.Datasource
	completionErr   error
	resolveCalls    int
	findCalls       int
	completionCalls int
}

func (r *publishResolver) ResolveProject(context.Context, identity.Selector) (datasourcepublish.Project, error) {
	r.resolveCalls++
	return r.project, nil
}

func (r *publishResolver) FindDatasources(context.Context, string, string) ([]datasourcepublish.Datasource, error) {
	r.findCalls++
	return append([]datasourcepublish.Datasource(nil), r.collisions...), nil
}

func (r *publishResolver) ResolvePublishedDatasource(context.Context, string, string) (datasourcepublish.Datasource, error) {
	r.completionCalls++
	return r.completion, r.completionErr
}

type preparedPublish struct{ committed bool }

func (p *preparedPublish) Commit(context.Context) (datasourcepublish.Result, error) {
	p.committed = true
	return datasourcepublish.Result{Status: "succeeded", DatasourceLUID: "ds-new", DatasourceName: "Sales", ProjectLUID: "project-1"}, nil
}

type publisher struct {
	calls    int
	request  datasourcepublish.PublishRequest
	prepared *preparedPublish
}

func (p *publisher) Prepare(_ context.Context, request datasourcepublish.PublishRequest) (datasourcepublish.PreparedPublish, error) {
	p.calls++
	p.request = request
	p.prepared = &preparedPublish{}
	return p.prepared, nil
}

func composedArtifact() datasourcepublish.Artifact {
	return datasourcepublish.Artifact{
		Path: "artifacts/datasource/Sales", PayloadPath: "Sales.tdsx", Filename: "Sales.tdsx", Size: 42,
		Name: "Sales", TableauID: "ds-source", Fingerprint: "sha256:abc", SourceEnvironment: "dev", SourceSite: "sandbox",
		SourceProjectName: "Analytics", SourceProjectID: "project-1", CompositionStatus: "composed",
		ParentDataSourceURLs: []string{"parent-a", "parent-b"},
	}
}

func TestPublishPreviewDoesNotUploadAndIncludesExactParents(t *testing.T) {
	artifacts := &publishArtifactReader{artifact: composedArtifact()}
	resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}}
	publisher := &publisher{}
	output, err := datasourcepublish.New(artifacts, resolver, publisher).Execute(context.Background(), datasourcepublish.Input{
		ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox",
		ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || publisher.calls != 0 || output.Plan.CompositionStatus != "composed" || !reflect.DeepEqual(output.Plan.ParentDataSourceURLs, []string{"parent-a", "parent-b"}) {
		t.Fatalf("output = %#v, publisher = %#v", output, publisher)
	}
}

func TestPublishDefaultSiteRequiresResolvedTarget(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		artifacts := &publishArtifactReader{artifact: composedArtifact()}
		resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}}
		publisher := &publisher{}
		out, err := datasourcepublish.New(artifacts, resolver, publisher).Execute(context.Background(), datasourcepublish.Input{
			ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", TargetResolved: resolved,
			ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate,
		}, true)
		if (err == nil) != resolved || publisher.calls != 0 {
			t.Fatalf("resolved=%t output=%#v err=%v publisher=%#v", resolved, out, err, publisher)
		}
		if !resolved && (artifacts.calls != 0 || resolver.resolveCalls != 0) {
			t.Fatalf("unresolved target reached readers: artifacts=%d projects=%d", artifacts.calls, resolver.resolveCalls)
		}
	}
}

func TestPublishRevalidatesThenForwardsExactParents(t *testing.T) {
	artifacts := &publishArtifactReader{artifact: composedArtifact()}
	resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}}
	publisher := &publisher{}
	output, err := datasourcepublish.New(artifacts, resolver, publisher).Execute(context.Background(), datasourcepublish.Input{
		ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox",
		ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate, AsJob: true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || artifacts.calls != 2 || resolver.resolveCalls != 2 || resolver.findCalls != 2 || publisher.calls != 1 || !publisher.prepared.committed {
		t.Fatalf("output = %#v, artifacts = %#v, resolver = %#v, publisher = %#v", output, artifacts, resolver, publisher)
	}
	if publisher.request.Mode != datasourcepublish.ModeCreate || !reflect.DeepEqual(publisher.request.ParentDataSourceURLs, []string{"parent-a", "parent-b"}) {
		t.Fatalf("request = %#v", publisher.request)
	}
	if !publisher.request.AsJob || !output.Plan.AsJob {
		t.Fatalf("async plan/request = %#v / %#v", output.Plan, publisher.request)
	}
}

func TestPublishUnknownAsyncOutcomePreservesJobAndDisablesRetryAdvice(t *testing.T) {
	artifacts := &publishArtifactReader{artifact: composedArtifact()}
	resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}}
	publisher := &outcomePublisher{result: datasourcepublish.Result{Status: "timed_out", JobID: "job-1", TableauRequestID: "poll-request"}, err: errors.New("poll timeout")}
	output, err := datasourcepublish.New(artifacts, resolver, publisher).Execute(context.Background(), datasourcepublish.Input{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate, AsJob: true}, false)
	var structured *errs.Error
	if err == nil || output.Result == nil || output.Result.JobID != "job-1" || !errors.As(err, &structured) || structured.ID != "datasource.publish.outcome_unknown" || structured.Phase != errs.PhaseSubmission || structured.Outcome != errs.OutcomeUnknown || structured.TableauJobID != "job-1" || structured.TableauRequestID != "poll-request" || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishResolvesAuthoritativeIdentityAfterCompletedJobOmitsIt(t *testing.T) {
	artifacts := &publishArtifactReader{artifact: composedArtifact()}
	resolver := &publishResolver{
		project:    datasourcepublish.Project{LUID: "project-1", Path: "Analytics"},
		completion: datasourcepublish.Datasource{LUID: "ds-new", Name: "Sales", ProjectLUID: "project-1"},
	}
	publisher := &outcomePublisher{result: datasourcepublish.Result{Status: "succeeded", JobID: "job-1", TableauRequestID: "poll-request"}}
	output, err := datasourcepublish.New(artifacts, resolver, publisher).Execute(context.Background(), datasourcepublish.Input{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate, AsJob: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || output.Result.Status != "succeeded" || output.Result.DatasourceLUID != "ds-new" || output.Result.DatasourceName != "Sales" || output.Result.ProjectLUID != "project-1" || output.Result.JobID != "job-1" || output.Result.TableauRequestID != "poll-request" || resolver.completionCalls != 1 {
		t.Fatalf("output = %#v, completion calls = %d", output, resolver.completionCalls)
	}
}

func TestPublishKeepsCompletedJobOutcomeUnknownWhenIdentityCannotBeResolved(t *testing.T) {
	tests := []struct {
		name       string
		completion datasourcepublish.Datasource
		err        error
	}{
		{name: "missing", err: errors.New("completed datasource was not visible before the resolution deadline")},
		{name: "ambiguous", err: errors.New("completed datasource resolution returned multiple matches")},
		{name: "incomplete", completion: datasourcepublish.Datasource{Name: "Sales", ProjectLUID: "project-1"}},
		{name: "wrong name", completion: datasourcepublish.Datasource{LUID: "ds-new", Name: "Other", ProjectLUID: "project-1"}},
		{name: "wrong project", completion: datasourcepublish.Datasource{LUID: "ds-new", Name: "Sales", ProjectLUID: "other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}, completion: test.completion, completionErr: test.err}
			publisher := &outcomePublisher{result: datasourcepublish.Result{Status: "succeeded", JobID: "job-1", TableauRequestID: "poll-request"}}
			output, err := datasourcepublish.New(&publishArtifactReader{artifact: composedArtifact()}, resolver, publisher).Execute(context.Background(), datasourcepublish.Input{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate, AsJob: true}, false)
			var structured *errs.Error
			if err == nil || output.Result == nil || output.Result.JobID != "job-1" || !errors.As(err, &structured) || structured.ID != "datasource.publish.outcome_unknown" || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeUnknown || structured.TableauJobID != "job-1" || structured.TableauRequestID != "poll-request" || structured.Retryable == nil || *structured.Retryable || resolver.completionCalls != 1 {
				t.Fatalf("error = %#v, completion calls = %d", err, resolver.completionCalls)
			}
		})
	}
}

type outcomePublisher struct {
	result datasourcepublish.Result
	err    error
}

func (p *outcomePublisher) Prepare(context.Context, datasourcepublish.PublishRequest) (datasourcepublish.PreparedPublish, error) {
	return outcomePrepared{result: p.result, err: p.err}, nil
}

type outcomePrepared struct {
	result datasourcepublish.Result
	err    error
}

func (p outcomePrepared) Commit(context.Context) (datasourcepublish.Result, error) {
	return p.result, p.err
}

func TestPublishRequiresExplicitModeAndRejectsCollision(t *testing.T) {
	artifact := composedArtifact()
	resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}, collisions: []datasourcepublish.Datasource{{LUID: "existing", Name: "Sales", ProjectLUID: "project-1"}}}
	action := datasourcepublish.New(&publishArtifactReader{artifact: artifact}, resolver, &publisher{})
	_, err := action.Execute(context.Background(), datasourcepublish.Input{ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}}, false)
	if err == nil {
		t.Fatal("missing mode succeeded")
	}
	_, err = action.Execute(context.Background(), datasourcepublish.Input{ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeCreate}, false)
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.publish.conflict" {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishSourceDefaultRequiresRecordedLUIDToRemainExact(t *testing.T) {
	artifact := composedArtifact()
	resolver := &publishResolver{project: datasourcepublish.Project{LUID: "project-1", Path: "Analytics"}, collisions: []datasourcepublish.Datasource{{LUID: "replacement", Name: "Sales", ProjectLUID: "project-1"}}}
	_, err := datasourcepublish.New(&publishArtifactReader{artifact: artifact}, resolver, &publisher{}).Execute(context.Background(), datasourcepublish.Input{
		ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", SourceDefaulted: true, Mode: datasourcepublish.ModeOverwrite,
	}, false)
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.publish.source_changed" {
		t.Fatalf("error = %#v", err)
	}
}
