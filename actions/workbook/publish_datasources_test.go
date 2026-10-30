package workbook_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
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
	{LUID: "ds-name", Name: "Sales & Margin", ContentURL: "SalesMargin_1", ProjectLUID: "project-1", ProjectName: "Finance", OwnerLUID: "user-1", UpdatedAt: "2026-01-02T03:04:05Z"},
	{LUID: "ds-similar", Name: "Sales & Margin v2", ContentURL: "SalesMarginV2", ProjectLUID: "project-1", ProjectName: "Finance"},
	{LUID: "ds-suffix", Name: "Archive", ContentURL: "SalesMargin_12", ProjectLUID: "project-2", ProjectName: "Archive"},
	{LUID: "ds-orders", Name: "Orders", ContentURL: "Orders", ProjectLUID: "project-1", ProjectName: "Finance", OwnerLUID: "user-1", UpdatedAt: "2026-01-03T00:00:00Z"},
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
		datasources: map[string]value.Datasource{"ds-name": publishBindingSite[0], "ds-orders": publishBindingSite[3]},
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
	if !reflect.DeepEqual(out.Plan.DatasourceBindings, wantPlan) || len(out.Plan.Warnings) != 0 {
		t.Fatalf("plan bindings = %#v warnings = %#v", out.Plan.DatasourceBindings, out.Plan.Warnings)
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
	datasources := append([]workbookops.PublishDatasource{
		{Caption: "Orders history", ContentURL: "OrdersArchive"},
		{Caption: "Orders history", ContentURL: "OrdersCurrent"},
	}, publishBindingDatasources...)
	for _, test := range []struct {
		name     string
		bindings []workbookops.DatasourceBinding
		want     string
	}{
		{"no match", []workbookops.DatasourceBinding{{Source: "Sales", TargetLUID: "ds-name"}}, `"Sales" matches no published data source`},
		{"similar name", []workbookops.DatasourceBinding{{Source: "sales & margin", TargetLUID: "ds-name"}}, "matches no published data source"},
		{"wildcard is literal", []workbookops.DatasourceBinding{{Source: "Orders*", TargetLUID: "ds-orders"}}, "matches no published data source"},
		{"ambiguous caption", []workbookops.DatasourceBinding{{Source: "Orders history", TargetLUID: "ds-orders"}}, "OrdersArchive, OrdersCurrent"},
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

func TestPublishPlanResolvesDatasourceBindingsLiterally(t *testing.T) {
	for _, test := range []struct {
		name, source, want string
		datasources        []workbookops.PublishDatasource
	}{
		{
			name: "content URL takes precedence over duplicate captions", source: "Orders", want: "Orders",
			datasources: []workbookops.PublishDatasource{{Caption: "Orders", ContentURL: "OrdersArchive"}, {Caption: "Orders", ContentURL: "Orders"}},
		},
		{
			name: "caption metacharacters are literal", source: "Orders*[?]", want: "LiteralOrders",
			datasources: []workbookops.PublishDatasource{{Caption: "Orders*[?]", ContentURL: "LiteralOrders"}, {Caption: "OrdersArchive", ContentURL: "Archive"}},
		},
		{
			name: "content URL metacharacters are literal", source: "Orders*[?]", want: "Orders*[?]",
			datasources: []workbookops.PublishDatasource{{Caption: "Orders", ContentURL: "Orders*[?]"}, {Caption: "Orders*[?]", ContentURL: "Archive"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			publisher := &publishPublisher{}
			action := workbookops.NewPublish(
				publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifact", Filename: "Finance.twbx", Name: "Finance", Datasources: test.datasources}},
				publishBindingResolver(), publisher,
			)
			plan, err := action.Plan(t.Context(), publishBindingInput(workbookops.DatasourceBinding{Source: test.source, TargetLUID: "ds-name"}))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := action.Apply(t.Context(), plan); err != nil {
				t.Fatal(err)
			}
			if want := map[string]string{test.want: "SalesMargin_1"}; !reflect.DeepEqual(publisher.input.DatasourceContentURLs, want) {
				t.Fatalf("upload rewrite = %#v, want %#v", publisher.input.DatasourceContentURLs, want)
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

func TestPublishPreviewLooksUpNoDatasources(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.contentURLErr, resolver.nameErr = errors.New("preview looked up content URLs"), errors.New("preview looked up names")
	out, err := publishBindingAction(&publishPublisher{}, resolver).Execute(context.Background(), publishBindingInput(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Plan.Warnings) != 0 || len(out.Plan.DatasourceBindings) != 3 {
		t.Fatalf("plan = %#v", out.Plan)
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
		`  - "Sales & Margin", content URL SalesMargin_1, LUID ds-name, project "Finance" (LUID project-1), owner user-1, updated 2026-01-02T03:04:05Z. Remap: ` + remap,
		"- Returns (content URL Returns):",
		"  No candidates found. Publish the data source if it does not exist yet. If you already published it, find it with " + byProject + " or " + all + "; there is no partial-name filter, and --name matches exact names only.",
		"Always check a candidate before remapping.",
	}, "\n")
	if structured.CorrectiveAction != want {
		t.Fatalf("corrective action:\n%s\nwant:\n%s", structured.CorrectiveAction, want)
	}
	if payload := errs.Structure(err).Error; payload.UpstreamCode != "400011" || payload.UpstreamStatus != 400 {
		t.Fatalf("upstream evidence = %#v", payload)
	}
}

func TestPublishApplyBoundsCandidatesPerReference(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.site = nil
	for index := range 7 {
		resolver.site = append(resolver.site, value.Datasource{LUID: fmt.Sprintf("ds-%d", index), Name: "Returns", ContentURL: fmt.Sprintf("Returns_%d", index), ProjectLUID: "project-1", ProjectName: "Finance"})
	}
	action := workbookops.NewPublish(
		publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifacts/workbook/Finance", Filename: "Finance.twbx", Name: "Finance", Datasources: publishBindingDatasources[2:]}},
		resolver, &publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}},
	)
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || strings.Count(structured.CorrectiveAction, "Remap: ") != 5 || strings.Contains(structured.CorrectiveAction, "LUID ds-5") {
		t.Fatalf("error = %#v", err)
	}
	more := "  2 more data source(s) have this name; list them with " + commandhint.Environment("production", "content", "datasource", "list", "--name", "Returns") + "."
	if !strings.Contains(structured.CorrectiveAction, more) || strings.Contains(structured.CorrectiveAction, "No candidates found") {
		t.Fatalf("corrective action:\n%s", structured.CorrectiveAction)
	}
}

func TestPublishApplyReturnsTableauRejectionWhenEveryReferenceExists(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.nameErr = errors.New("name lookup must not run")
	action := workbookops.NewPublish(
		publishArtifactReader{artifact: workbookops.PublishArtifact{Path: "artifacts/workbook/Finance", Filename: "Finance.twbx", Name: "Finance", Datasources: publishBindingDatasources[1:2]}},
		resolver, &publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}},
	)
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workbook.publish.failed" || structured.Outcome != errs.OutcomeNotApplied || errs.Structure(err).Error.UpstreamCode != "400011" {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishApplyNotesContentURLLookupFailureOnTableauRejection(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.contentURLErr = errors.New("lookup unavailable")
	action := publishBindingAction(&publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}}, resolver)
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "workbook.publish.failed" || structured.Outcome != errs.OutcomeNotApplied || !strings.HasSuffix(structured.CorrectiveAction, " The workbook's data sources could not be checked on this site: lookup unavailable.") {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublishApplyReportsCandidateSearchFailureInRejection(t *testing.T) {
	resolver := publishBindingResolver()
	resolver.nameErr = errors.New("list unavailable")
	action := publishBindingAction(&publishPublisher{err: publishUpstreamError{status: 400, code: "400011"}}, resolver)
	_, err := action.Execute(context.Background(), publishBindingInput(), false)
	var structured *errs.Error
	if !errors.As(err, &structured) || !strings.HasPrefix(structured.Summary, "Your workbook contains 2 data source(s)") || strings.Count(structured.CorrectiveAction, "Candidate search was unavailable: list unavailable.") != 2 {
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

func (r publishResolver) FindDatasourcesByContentURL(_ context.Context, contentURLs []string) ([]value.Datasource, error) {
	if r.contentURLErr != nil {
		return nil, r.contentURLErr
	}
	return siteDatasources(r.site, func(item value.Datasource) bool { return slices.Contains(contentURLs, item.ContentURL) }), nil
}

func (r publishResolver) FindDatasourcesByName(_ context.Context, names []string) ([]value.Datasource, error) {
	if r.nameErr != nil {
		return nil, r.nameErr
	}
	return siteDatasources(r.site, func(item value.Datasource) bool { return slices.Contains(names, item.Name) }), nil
}

func siteDatasources(site []value.Datasource, match func(value.Datasource) bool) []value.Datasource {
	var matches []value.Datasource
	for _, item := range site {
		if match(item) {
			matches = append(matches, item)
		}
	}
	return matches
}

func (*publishChangingResolver) ReadDatasource(context.Context, string) (value.Datasource, error) {
	return value.Datasource{}, errors.New("unexpected datasource read")
}

func (*publishChangingResolver) FindDatasourcesByContentURL(context.Context, []string) ([]value.Datasource, error) {
	return nil, nil
}

func (*publishChangingResolver) FindDatasourcesByName(context.Context, []string) ([]value.Datasource, error) {
	return nil, nil
}

func (*publishRevalidationResolver) ReadDatasource(context.Context, string) (value.Datasource, error) {
	return value.Datasource{}, errors.New("unexpected datasource read")
}

func (*publishRevalidationResolver) FindDatasourcesByContentURL(context.Context, []string) ([]value.Datasource, error) {
	return nil, nil
}

func (*publishRevalidationResolver) FindDatasourcesByName(context.Context, []string) ([]value.Datasource, error) {
	return nil, nil
}
