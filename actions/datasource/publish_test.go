package datasource_test

import (
	"bytes"
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPublishDatasourcePublishOutputGoldens(t *testing.T) {
	value := datasourceops.PublishOutput{Plan: datasourceops.PublishPlan{Mode: "execute", PublishMode: datasourceops.ModeOverwrite, Operation: "datasource.publish", ArtifactPath: "artifacts/datasource/Sales", ArtifactFingerprint: "sha256:secret-diagnostic", Filename: "Sales.tds", DatasourceName: "Sales", CompositionStatus: "composed", ParentDataSourceURLs: []string{"parent-a", "parent-b"}, Target: datasourceops.PublishTarget{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1", ProjectPath: "Analytics", ExistingLUID: "ds-1"}, Substeps: []string{"resolve exact destination", "poll asynchronous job"}, AsJob: true}, Result: &datasourceops.PublishResult{Status: "succeeded", DatasourceLUID: "ds-1", DatasourceName: "Sales", ProjectLUID: "project-1", JobID: "job-1", TableauRequestID: "request-1"}, Help: []string{"tadx content datasource inspect --id ds-1"}}
	publishAssertDatasourcePublishGolden(t, "compact.toon", value, false)
	publishAssertDatasourcePublishGolden(t, "full.toon", value, true)
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

func publishAssertDatasourcePublishGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var actual bytes.Buffer
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata/publish", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(actual.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, actual.Bytes())
	}
}

type publishPublishArtifactReader struct {
	artifact datasourceops.PublishArtifact
	calls    int
}

func (r *publishPublishArtifactReader) ReadDatasource(context.Context, string) (datasourceops.PublishArtifact, error) {
	r.calls++
	return r.artifact, nil
}

type publishPublishResolver struct {
	project         datasourceops.Project
	collisions      []datasourceops.Record
	completion      datasourceops.Record
	completionErr   error
	resolveCalls    int
	findCalls       int
	completionCalls int
}

func (r *publishPublishResolver) ResolveProject(context.Context, identity.Selector) (datasourceops.Project, error) {
	r.resolveCalls++
	return r.project, nil
}

func (r *publishPublishResolver) FindDatasources(context.Context, string, string) ([]datasourceops.Record, error) {
	r.findCalls++
	return append([]datasourceops.Record(nil), r.collisions...), nil
}

func (r *publishPublishResolver) ResolvePublishedDatasource(context.Context, string, string) (datasourceops.Record, error) {
	r.completionCalls++
	return r.completion, r.completionErr
}

type publishPreparedPublish struct{ committed bool }

func (p *publishPreparedPublish) Commit(context.Context) (datasourceops.PublishResult, error) {
	p.committed = true
	return datasourceops.PublishResult{Status: "succeeded", DatasourceLUID: "ds-new", DatasourceName: "Sales", ProjectLUID: "project-1"}, nil
}

type publishPublisher struct {
	calls    int
	request  datasourceops.PublishRequest
	prepared *publishPreparedPublish
}

func (p *publishPublisher) Prepare(_ context.Context, request datasourceops.PublishRequest) (datasourceops.PreparedPublish, error) {
	p.calls++
	p.request = request
	p.prepared = &publishPreparedPublish{}
	return p.prepared, nil
}

func publishComposedArtifact() datasourceops.PublishArtifact {
	return datasourceops.PublishArtifact{
		Path: "artifacts/datasource/Sales", PayloadPath: "Sales.tdsx", Filename: "Sales.tdsx", Size: 42,
		Name: "Sales", TableauID: "ds-source", Fingerprint: "sha256:abc", SourceEnvironment: "dev", SourceSite: "sandbox",
		SourceProjectName: "Analytics", SourceProjectID: "project-1", CompositionStatus: "composed",
		ParentDataSourceURLs: []string{"parent-a", "parent-b"},
	}
}

func TestPublishPublishPreviewDoesNotUploadAndIncludesExactParents(t *testing.T) {
	artifacts := &publishPublishArtifactReader{artifact: publishComposedArtifact()}
	resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}}
	publisher := &publishPublisher{}
	output, err := datasourceops.NewPublish(artifacts, resolver, publisher).Execute(context.Background(), datasourceops.PublishInput{
		ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox",
		ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || publisher.calls != 0 || output.Plan.CompositionStatus != "composed" || !reflect.DeepEqual(output.Plan.ParentDataSourceURLs, []string{"parent-a", "parent-b"}) {
		t.Fatalf("output = %#v, publisher = %#v", output, publisher)
	}
}

func TestPublishPublishDefaultSiteRequiresResolvedTarget(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		artifacts := &publishPublishArtifactReader{artifact: publishComposedArtifact()}
		resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}}
		publisher := &publishPublisher{}
		out, err := datasourceops.NewPublish(artifacts, resolver, publisher).Execute(context.Background(), datasourceops.PublishInput{
			ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", TargetResolved: resolved,
			ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate,
		}, true)
		if (err == nil) != resolved || publisher.calls != 0 {
			t.Fatalf("resolved=%t output=%#v err=%v publisher=%#v", resolved, out, err, publisher)
		}
		if !resolved && (artifacts.calls != 0 || resolver.resolveCalls != 0) {
			t.Fatalf("unresolved target reached readers: artifacts=%d projects=%d", artifacts.calls, resolver.resolveCalls)
		}
	}
}

func TestPublishPublishRevalidatesThenForwardsExactParents(t *testing.T) {
	artifacts := &publishPublishArtifactReader{artifact: publishComposedArtifact()}
	resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}}
	publisher := &publishPublisher{}
	output, err := datasourceops.NewPublish(artifacts, resolver, publisher).Execute(context.Background(), datasourceops.PublishInput{
		ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox",
		ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate, AsJob: true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || artifacts.calls != 2 || resolver.resolveCalls != 2 || resolver.findCalls != 2 || publisher.calls != 1 || !publisher.prepared.committed {
		t.Fatalf("output = %#v, artifacts = %#v, resolver = %#v, publisher = %#v", output, artifacts, resolver, publisher)
	}
	if publisher.request.Mode != datasourceops.ModeCreate || !reflect.DeepEqual(publisher.request.ParentDataSourceURLs, []string{"parent-a", "parent-b"}) {
		t.Fatalf("request = %#v", publisher.request)
	}
	if !publisher.request.AsJob || !output.Plan.AsJob {
		t.Fatalf("async plan/request = %#v / %#v", output.Plan, publisher.request)
	}
}

func TestPublishPublishUnknownAsyncOutcomePreservesJobAndDisablesRetryAdvice(t *testing.T) {
	artifacts := &publishPublishArtifactReader{artifact: publishComposedArtifact()}
	resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}}
	publisher := &publishOutcomePublisher{result: datasourceops.PublishResult{Status: "timed_out", JobID: "job-1", TableauRequestID: "poll-request"}, err: errors.New("poll timeout")}
	output, err := datasourceops.NewPublish(artifacts, resolver, publisher).Execute(context.Background(), datasourceops.PublishInput{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate, AsJob: true}, false)
	var structured *errs.Error
	if err == nil || output.Result == nil || output.Result.JobID != "job-1" || !errors.As(err, &structured) || structured.ID != "datasource.publish.outcome_unknown" || structured.Phase != errs.PhaseSubmission || structured.Outcome != errs.OutcomeUnknown || structured.TableauJobID != "job-1" || structured.TableauRequestID != "poll-request" || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishPublishResolvesAuthoritativeIdentityAfterCompletedJobOmitsIt(t *testing.T) {
	artifacts := &publishPublishArtifactReader{artifact: publishComposedArtifact()}
	resolver := &publishPublishResolver{
		project:    datasourceops.Project{LUID: "project-1", Path: "Analytics"},
		completion: datasourceops.Record{LUID: "ds-new", Name: "Sales", ProjectLUID: "project-1"},
	}
	publisher := &publishOutcomePublisher{result: datasourceops.PublishResult{Status: "succeeded", JobID: "job-1", TableauRequestID: "poll-request"}}
	output, err := datasourceops.NewPublish(artifacts, resolver, publisher).Execute(context.Background(), datasourceops.PublishInput{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate, AsJob: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || output.Result.Status != "succeeded" || output.Result.DatasourceLUID != "ds-new" || output.Result.DatasourceName != "Sales" || output.Result.ProjectLUID != "project-1" || output.Result.JobID != "job-1" || output.Result.TableauRequestID != "poll-request" || resolver.completionCalls != 1 {
		t.Fatalf("output = %#v, completion calls = %d", output, resolver.completionCalls)
	}
}

func TestPublishPublishTreatsDatasourceIndexDelayAsConfirmedPending(t *testing.T) {
	resolver := &publishPublishResolver{
		project:       datasourceops.Project{LUID: "project-1", Path: "Analytics"},
		collisions:    []datasourceops.Record{{LUID: "ds-old", Name: "Sales", ProjectLUID: "project-1"}},
		completionErr: datasourceops.PublishErrPublishedDatasourceNotVisible,
	}
	publisher := &publishOutcomePublisher{result: datasourceops.PublishResult{Status: "succeeded", JobID: "job-1", TableauRequestID: "poll-request"}}
	output, err := datasourceops.NewPublish(&publishPublishArtifactReader{artifact: publishComposedArtifact()}, resolver, publisher).Execute(t.Context(), datasourceops.PublishInput{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeOverwrite, AsJob: true}, false)
	if err != nil || output.Result == nil || output.Result.Status != "succeeded" || output.Result.Verification != "destination_pending" || output.Result.DatasourceLUID != "" || output.Result.JobID != "job-1" {
		t.Fatalf("output = %#v, err = %v", output, err)
	}
	if !strings.Contains(strings.Join(output.Help, " "), "tadx job inspect --id job-1") {
		t.Fatalf("missing exact recovery hint: %v", output.Help)
	}
}

func TestPublishPublishPreservesConfirmedCompletionWhenIdentityCannotBeResolved(t *testing.T) {
	tests := []struct {
		name       string
		completion datasourceops.Record
		err        error
	}{
		{name: "missing", err: errors.New("completed datasource was not visible before the resolution deadline")},
		{name: "ambiguous", err: errors.New("completed datasource resolution returned multiple matches")},
		{name: "incomplete", completion: datasourceops.Record{Name: "Sales", ProjectLUID: "project-1"}},
		{name: "wrong name", completion: datasourceops.Record{LUID: "ds-new", Name: "Other", ProjectLUID: "project-1"}},
		{name: "wrong project", completion: datasourceops.Record{LUID: "ds-new", Name: "Sales", ProjectLUID: "other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}, completion: test.completion, completionErr: test.err}
			publisher := &publishOutcomePublisher{result: datasourceops.PublishResult{Status: "succeeded", JobID: "job-1", TableauRequestID: "poll-request"}}
			output, err := datasourceops.NewPublish(&publishPublishArtifactReader{artifact: publishComposedArtifact()}, resolver, publisher).Execute(context.Background(), datasourceops.PublishInput{ArtifactPath: "artifacts/datasource/Sales", Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate, AsJob: true}, false)
			var structured *errs.Error
			if err == nil || output.Result == nil || output.Result.Status != "succeeded" || output.Result.Verification == "" || output.Result.JobID != "job-1" || !errors.As(err, &structured) || structured.ID != "datasource.publish.destination_unavailable" || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeConfirmed || structured.TableauJobID != "job-1" || structured.TableauRequestID != "poll-request" || structured.Retryable == nil || *structured.Retryable || resolver.completionCalls != 1 {
				t.Fatalf("error = %#v, completion calls = %d", err, resolver.completionCalls)
			}
		})
	}
}

type publishOutcomePublisher struct {
	result datasourceops.PublishResult
	err    error
}

func (p *publishOutcomePublisher) Prepare(context.Context, datasourceops.PublishRequest) (datasourceops.PreparedPublish, error) {
	return publishOutcomePrepared{result: p.result, err: p.err}, nil
}

type publishOutcomePrepared struct {
	result datasourceops.PublishResult
	err    error
}

func (p publishOutcomePrepared) Commit(context.Context) (datasourceops.PublishResult, error) {
	return p.result, p.err
}

func TestPublishPublishRequiresExplicitModeAndRejectsCollision(t *testing.T) {
	artifact := publishComposedArtifact()
	resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}, collisions: []datasourceops.Record{{LUID: "existing", Name: "Sales", ProjectLUID: "project-1"}}}
	action := datasourceops.NewPublish(&publishPublishArtifactReader{artifact: artifact}, resolver, &publishPublisher{})
	_, err := action.Execute(context.Background(), datasourceops.PublishInput{ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}}, false)
	if err == nil {
		t.Fatal("missing mode succeeded")
	}
	_, err = action.Execute(context.Background(), datasourceops.PublishInput{ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeCreate}, false)
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.publish.conflict" {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishPublishSourceDefaultRequiresRecordedLUIDToRemainExact(t *testing.T) {
	artifact := publishComposedArtifact()
	resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}, collisions: []datasourceops.Record{{LUID: "replacement", Name: "Sales", ProjectLUID: "project-1"}}}
	_, err := datasourceops.NewPublish(&publishPublishArtifactReader{artifact: artifact}, resolver, &publishPublisher{}).Execute(context.Background(), datasourceops.PublishInput{
		ArtifactPath: artifact.Path, Environment: "dev", Site: "sandbox", SourceDefaulted: true, Mode: datasourceops.ModeOverwrite,
	}, false)
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.publish.source_changed" {
		t.Fatalf("error = %#v", err)
	}
}
