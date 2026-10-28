package workbook_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
)

type publishUpstreamError struct {
	status int
	code   string
}

func (e publishUpstreamError) Error() string          { return "Tableau request failed" }
func (e publishUpstreamError) HTTPStatus() int        { return e.status }
func (e publishUpstreamError) TableauCode() string    { return e.code }
func (e publishUpstreamError) TableauSummary() string { return "Bad Request" }
func (e publishUpstreamError) TableauDetail() string {
	return "There was a problem publishing the file."
}

var publishBindingDatasources = []workbookops.PublishDatasource{
	{Caption: "Sales & Margin", ContentURL: "SalesMargin"},
	{Caption: "Orders", ContentURL: "Orders"},
	{Caption: "Returns", ContentURL: "Returns"},
}

var publishBindingSite = []value.Datasource{
	{LUID: "ds-name", Name: "Sales & Margin", ContentURL: "SalesMargin_1", ProjectName: "Finance", OwnerLUID: "user-1", UpdatedAt: "2026-01-02T03:04:05Z"},
	{LUID: "ds-similar", Name: "Sales & Margin v2", ContentURL: "SalesMarginV2", ProjectName: "Finance"},
	{LUID: "ds-suffix", Name: "Archive", ContentURL: "SalesMargin_12", ProjectName: "Archive", OwnerLUID: "user-2", UpdatedAt: "2025-12-01T00:00:00Z"},
	{LUID: "ds-prefix", Name: "Other", ContentURL: "SalesMarginX_1", ProjectName: "Finance"},
	{LUID: "ds-orders", Name: "Orders", ContentURL: "Orders", ProjectName: "Finance", OwnerLUID: "user-1", UpdatedAt: "2026-01-03T00:00:00Z"},
}

func publishBindingAction(publisher *publishPublisher, resolver publishResolver) *workbookops.Publisher {
	return workbookops.NewPublish(
		publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifacts/workbook/Finance", Filename: "Finance.twbx", Name: "Finance", Fingerprint: "sha256:abc", Datasources: publishBindingDatasources}},
		resolver, publisher,
	)
}

func publishBindingInput(bindings ...workbookops.DatasourceBinding) workbookops.PublishInput {
	return workbookops.PublishInput{ArtifactPath: "artifacts/workbook/Finance", WorkspaceName: "default", Environment: "production", Site: "marketing", ProjectSelector: identity.Selector{LUID: "project-1"}, DatasourceBindings: bindings}
}

func publishBindingResolver() publishResolver {
	return publishResolver{
		project:     workbookops.Project{LUID: "project-1", Path: "Ops"},
		datasources: map[string]value.Datasource{"ds-name": publishBindingSite[0], "ds-orders": publishBindingSite[4]},
		site:        publishBindingSite,
	}
}

func TestPublishPlanMapsDatasourceBindingsOntoTheUploadCopy(t *testing.T) {
	publisher := &publishPublisher{result: workbookops.PublishResult{Status: "succeeded", WorkbookLUID: "wb-new", WorkbookName: "Finance", ProjectLUID: "project-1"}}
	resolver := publishBindingResolver()
	action := publishBindingAction(publisher, resolver)
	out, err := action.Execute(context.Background(), publishBindingInput(
		workbookops.DatasourceBinding{Source: "Sales & Margin", TargetLUID: "ds-name"},
		workbookops.DatasourceBinding{Source: "SalesMargin", TargetLUID: "ds-name"},
	), false)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"SalesMargin": "SalesMargin_1"}; !reflect.DeepEqual(publisher.input.DatasourceContentURLs, want) {
		t.Fatalf("upload rewrite = %#v, want %#v", publisher.input.DatasourceContentURLs, want)
	}
	wantPlan := []workbookops.PublishDatasourceBinding{
		{Datasource: "Sales & Margin", ContentURL: "SalesMargin", Origin: "explicit", TargetContentURL: "SalesMargin_1", TargetLUID: "ds-name"},
		{Datasource: "Orders", ContentURL: "Orders", Origin: "workbook", TargetContentURL: "Orders"},
		{Datasource: "Returns", ContentURL: "Returns", Origin: "workbook", TargetContentURL: "Returns"},
	}
	if !reflect.DeepEqual(out.Plan.DatasourceBindings, wantPlan) || len(out.Plan.Warnings) != 0 || len(out.Plan.DatasourceCandidates) != 0 {
		t.Fatalf("plan bindings = %#v warnings = %#v candidates = %#v", out.Plan.DatasourceBindings, out.Plan.Warnings, out.Plan.DatasourceCandidates)
	}
	compact := out.CompactOutput().(workbookops.PublishCompactResult)
	if compact.Status != "succeeded" || !reflect.DeepEqual(compact.DatasourceBindings, wantPlan) {
		t.Fatalf("compact bindings = %#v", compact.DatasourceBindings)
	}
}

func TestPublishPlanWithoutBindingsLeavesEveryReferenceToTableau(t *testing.T) {
	publisher := &publishPublisher{}
	action := publishBindingAction(publisher, publishBindingResolver())
	plan, err := action.Plan(context.Background(), publishBindingInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := action.Apply(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if publisher.input.DatasourceContentURLs != nil || len(plan.DatasourceBindings) != 3 || plan.DatasourceBindings[0] != (workbookops.PublishDatasourceBinding{Datasource: "Sales & Margin", ContentURL: "SalesMargin", Origin: "workbook", TargetContentURL: "SalesMargin"}) {
		t.Fatalf("request = %#v bindings = %#v", publisher.input, plan.DatasourceBindings)
	}
}

func TestPublishPlanRejectsUnusableDatasourceBindings(t *testing.T) {
	datasources := append([]workbookops.PublishDatasource{{Caption: "Orders", ContentURL: "OrdersArchive"}}, publishBindingDatasources...)
	for _, test := range []struct {
		name     string
		bindings []workbookops.DatasourceBinding
		want     string
	}{
		{"no match", []workbookops.DatasourceBinding{{Source: "Sales", TargetLUID: "ds-name"}}, `"Sales" matches no published data source`},
		{"similar name", []workbookops.DatasourceBinding{{Source: "sales & margin", TargetLUID: "ds-name"}}, "matches no published data source"},
		{"ambiguous caption", []workbookops.DatasourceBinding{{Source: "Orders", TargetLUID: "ds-orders"}}, "OrdersArchive, Orders"},
		{"conflicting targets", []workbookops.DatasourceBinding{{Source: "Sales & Margin", TargetLUID: "ds-name"}, {Source: "SalesMargin", TargetLUID: "ds-orders"}}, "both"},
	} {
		t.Run(test.name, func(t *testing.T) {
			action := workbookops.NewPublish(
				publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifact", Filename: "Finance.twbx", Name: "Finance", Datasources: datasources}},
				publishResolver{projectErr: errors.New("remote resolution must not run")}, &publishPublisher{},
			)
			_, err := action.Plan(context.Background(), publishBindingInput(test.bindings...))
			payload := errs.Structure(err).Error
			if payload.Kind != errs.KindUsage || len(payload.Validation) != 1 || payload.Validation[0].Field != "datasource-binding" || !strings.Contains(payload.Summary, test.want) {
				t.Fatalf("error = %#v", payload)
			}
		})
	}
}

func TestPublishPlanReportsUnreadableBindingTarget(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.datasourceErr = publishUpstreamError{status: 404, code: "404004"}
	action := publishBindingAction(&publishPublisher{}, resolver)
	_, err := action.Plan(context.Background(), publishBindingInput(workbookops.DatasourceBinding{Source: "SalesMargin", TargetLUID: "ds-missing"}))
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workbook.publish.datasource_binding" || structured.Kind != errs.KindOperation || structured.Resource != "ds-missing" {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishPreviewListsExactDatasourceCandidates(t *testing.T) {
	action := publishBindingAction(&publishPublisher{}, publishBindingResolver())
	out, err := action.Execute(context.Background(), publishBindingInput(), true)
	if err != nil {
		t.Fatal(err)
	}
	remap := func(binding string) string {
		return commandhint.Target("production", "default", "content", "workbook", "publish", "--artifact", "artifacts/workbook/Finance", "--project-id", "project-1", "--name", "Finance", "--datasource-binding", binding)
	}
	want := []workbookops.PublishDatasourceCandidate{
		{SourceContentURL: "SalesMargin", Name: "Sales & Margin", ContentURL: "SalesMargin_1", LUID: "ds-name", Project: "Finance", OwnerLUID: "user-1", UpdatedAt: "2026-01-02T03:04:05Z", Command: remap("SalesMargin=ds-name")},
		{SourceContentURL: "SalesMargin", Name: "Archive", ContentURL: "SalesMargin_12", LUID: "ds-suffix", Project: "Archive", OwnerLUID: "user-2", UpdatedAt: "2025-12-01T00:00:00Z", Command: remap("SalesMargin=ds-suffix")},
		{SourceContentURL: "Orders", Name: "Orders", ContentURL: "Orders", LUID: "ds-orders", Project: "Finance", OwnerLUID: "user-1", UpdatedAt: "2026-01-03T00:00:00Z", Command: remap("Orders=ds-orders")},
	}
	if !reflect.DeepEqual(out.Plan.DatasourceCandidates, want) {
		t.Fatalf("candidates = %#v", out.Plan.DatasourceCandidates)
	}
	joined := strings.Join(out.Plan.Warnings, "\n")
	for _, url := range []string{"SalesMargin", "Returns"} {
		if !strings.Contains(joined, "(content URL "+url+"): no data source on this site has that URL; this publish will likely fail") {
			t.Fatalf("warnings missing %s: %#v", url, out.Plan.Warnings)
		}
	}
	if strings.Contains(joined, "content URL Orders)") || len(out.Plan.Warnings) != 2 {
		t.Fatalf("warnings = %#v", out.Plan.Warnings)
	}
}

func TestPublishPreviewWarnsWhenCandidateSearchIsUnavailable(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.listErr = errors.New("list unavailable")
	action := publishBindingAction(&publishPublisher{}, resolver)
	out, err := action.Execute(context.Background(), publishBindingInput(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Plan.Warnings) != 1 || !strings.Contains(out.Plan.Warnings[0], "list unavailable") || len(out.Plan.DatasourceCandidates) != 0 {
		t.Fatalf("warnings = %#v candidates = %#v", out.Plan.Warnings, out.Plan.DatasourceCandidates)
	}
}

func TestPublishApplyExplainsUnmappedDatasourceRejection(t *testing.T) {
	publisher := &publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}}
	action := publishBindingAction(publisher, publishBindingResolver())
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) {
		t.Fatalf("error = %T %v", err, err)
	}
	if structured.ID != "workbook.publish.datasource_unmapped" || structured.Outcome != errs.OutcomeNotApplied || structured.Phase != errs.PhaseSubmission || structured.Retryable == nil || *structured.Retryable {
		t.Fatalf("structured error = %#v", structured)
	}
	if want := "Your workbook contains 2 data source(s) that do not map to a published data source on this Tableau Site."; structured.Summary != want {
		t.Fatalf("summary = %q", structured.Summary)
	}
	remap := commandhint.Target("production", "default", "content", "workbook", "publish", "--artifact", "artifacts/workbook/Finance", "--project-id", "project-1", "--name", "Finance", "--datasource-binding", "SalesMargin=ds-name")
	byProject := commandhint.Environment("production", "content", "datasource", "list", "--project-name", "<leaf>")
	all := commandhint.Environment("production", "content", "datasource", "list", "--all")
	want := strings.Join([]string{
		"This can happen when there are naming conflicts on the Tableau Site and should be resolved by using --datasource-binding <source name or content URL>=<target LUID>, where the source name or content URL are the data sources in your workbook and the LUID is the LUID of the published data source on the Tableau Site. Here are the data sources that need mapping along with suggested candidates found by searching the Tableau Site:",
		"- Sales & Margin (content URL SalesMargin):",
		`  - "Sales & Margin", content URL SalesMargin_1, LUID ds-name, project "Finance", owner user-1, updated 2026-01-02T03:04:05Z. Remap: ` + remap,
		"  - \"Archive\", content URL SalesMargin_12, LUID ds-suffix, project \"Archive\", owner user-2, updated 2025-12-01T00:00:00Z. Remap: " + strings.Replace(remap, "SalesMargin=ds-name", "SalesMargin=ds-suffix", 1),
		"- Returns (content URL Returns):",
		"  No candidates found. Publish the data source if it does not exist yet. If you already published it, find it with " + byProject + " or " + all + "; there is no partial-name filter, and --name matches exact names only.",
		"Candidates match by exact name, or by the workbook's content URL followed by _<digits>. Always check a candidate before remapping.",
	}, "\n")
	if structured.CorrectiveAction != want {
		t.Fatalf("corrective action:\n%s\nwant:\n%s", structured.CorrectiveAction, want)
	}
	if payload := errs.Structure(err).Error; payload.UpstreamCode != "400011" || payload.UpstreamStatus != 400 {
		t.Fatalf("upstream evidence = %#v", payload)
	}
}

func TestPublishApplyListsEveryUnmappedReferenceWhenNoneIsMissing(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.site = []value.Datasource{{LUID: "ds-orders", Name: "Orders", ContentURL: "Orders", ProjectName: "Finance"}}
	action := workbookops.NewPublish(
		publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifacts/workbook/Finance", Filename: "Finance.twbx", Name: "Finance", Datasources: publishBindingDatasources[1:2]}},
		resolver, &publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}},
	)
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || !strings.HasPrefix(structured.Summary, "Your workbook contains 1 data source(s)") || !strings.Contains(structured.CorrectiveAction, "LUID ds-orders") {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishApplyReportsCandidateSearchFailureInRejection(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.listErr = errors.New("list unavailable")
	action := publishBindingAction(&publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}}, resolver)
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || !strings.HasPrefix(structured.Summary, "Your workbook contains 3 data source(s)") || !strings.Contains(structured.CorrectiveAction, "Candidate search was unavailable: list unavailable.") {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishApplyReportsDefinitiveRejectionAsNotApplied(t *testing.T) {
	for _, test := range []struct {
		name        string
		datasources []workbookops.PublishDatasource
		err         error
		bindings    []workbookops.DatasourceBinding
	}{
		{"forbidden", publishBindingDatasources, publishUpstreamError{status: 403, code: "403004"}, nil},
		{"rejected without datasource references", nil, publishUpstreamError{status: 400, code: "400011"}, nil},
		{"rejected with every reference mapped", publishBindingDatasources[:1], publishUpstreamError{status: 400, code: "400011"}, []workbookops.DatasourceBinding{{Source: "SalesMargin", TargetLUID: "ds-name"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			action := workbookops.NewPublish(
				publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifact", Filename: "Finance.twbx", Name: "Finance", Datasources: test.datasources}},
				publishBindingResolver(), &publishPublisher{err: test.err},
			)
			_, err := action.Execute(context.Background(), publishBindingInput(test.bindings...), false)
			var structured *errs.Error
			if !errors.As(err, &structured) || structured.ID != "workbook.publish.failed" || structured.Outcome != errs.OutcomeNotApplied || structured.Phase != errs.PhaseSubmission {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func failedPublishJob() (workbookops.PublishResult, error) {
	return workbookops.PublishResult{Status: "failed", JobID: "job-1"}, &errs.Error{ID: "workbook.publish.monitor", Kind: errs.KindOperation, Operation: "workbook.publish", Summary: "Tableau reports that publication failed.", Cause: errors.New("remote job is failed"), Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown, TableauJobID: "job-1", Retryable: new(false)}
}

func TestPublishApplyExplainsFailedPublishJobWithMissingDatasource(t *testing.T) {
	result, monitor := failedPublishJob()
	action := publishBindingAction(&publishPublisher{result: result, err: monitor}, publishBindingResolver())
	out, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workbook.publish.datasource_unmapped" || structured.Outcome != errs.OutcomeNotApplied || structured.Phase != errs.PhaseSubmission || structured.TableauJobID != "job-1" || !errors.Is(err, monitor) {
		t.Fatalf("error = %#v", err)
	}
	if want := "Your workbook contains 2 data source(s) that do not map to a published data source on this Tableau Site."; structured.Summary != want {
		t.Fatalf("summary = %q", structured.Summary)
	}
	for _, want := range []string{"- Sales & Margin (content URL SalesMargin):", "LUID ds-name", "- Returns (content URL Returns):", datasourceCandidateRuleText} {
		if !strings.Contains(structured.CorrectiveAction, want) {
			t.Fatalf("corrective action missing %q:\n%s", want, structured.CorrectiveAction)
		}
	}
	if strings.Contains(structured.CorrectiveAction, "- Orders") || out.Result == nil || out.Result.Status != "failed" || out.Result.JobID != "job-1" {
		t.Fatalf("corrective action = %s result = %#v", structured.CorrectiveAction, out.Result)
	}
}

func TestPublishApplyKeepsFailedPublishJobWithoutMissingDatasourceEvidence(t *testing.T) {
	for _, test := range []struct {
		name        string
		datasources []workbookops.PublishDatasource
		listErr     error
	}{
		{"every reference has its URL on the site", publishBindingDatasources[1:2], nil},
		{"no datasource references", nil, nil},
		{"candidate search unavailable", publishBindingDatasources, errors.New("list unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, monitor := failedPublishJob()
			resolver := publishBindingResolver()
			resolver.listErr = test.listErr
			action := workbookops.NewPublish(
				publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifacts/workbook/Finance", Filename: "Finance.twbx", Name: "Finance", Datasources: test.datasources}},
				resolver, &publishPublisher{result: result, err: monitor},
			)
			_, err := action.Execute(context.Background(), publishBindingInput(), false)
			if err != monitor {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

const datasourceCandidateRuleText = "Candidates match by exact name, or by the workbook's content URL followed by _<digits>. Always check a candidate before remapping."

func (r publishResolver) ReadDatasource(_ context.Context, luid string) (value.Datasource, error) {
	if r.datasourceErr != nil {
		return value.Datasource{}, r.datasourceErr
	}
	item, ok := r.datasources[luid]
	if !ok {
		return value.Datasource{}, errors.New("unexpected datasource read " + luid)
	}
	return item, nil
}

func (r publishResolver) ListDatasources(_ context.Context, state paging.State) (paging.Page[value.Datasource], error) {
	if r.listErr != nil {
		return paging.Page[value.Datasource]{}, r.listErr
	}
	start := min((state.Number-1)*state.Size, len(r.site))
	end := min(start+state.Size, len(r.site))
	return paging.Page[value.Datasource]{Number: state.Number, Size: state.Size, Total: len(r.site), Items: r.site[start:end]}, nil
}

func (*publishChangingResolver) ReadDatasource(context.Context, string) (value.Datasource, error) {
	return value.Datasource{}, errors.New("unexpected datasource read")
}

func (*publishChangingResolver) ListDatasources(_ context.Context, state paging.State) (paging.Page[value.Datasource], error) {
	return paging.Page[value.Datasource]{Number: state.Number, Size: state.Size}, nil
}

func (*publishRevalidationResolver) ReadDatasource(context.Context, string) (value.Datasource, error) {
	return value.Datasource{}, errors.New("unexpected datasource read")
}

func (*publishRevalidationResolver) ListDatasources(_ context.Context, state paging.State) (paging.Page[value.Datasource], error) {
	return paging.Page[value.Datasource]{Number: state.Number, Size: state.Size}, nil
}
