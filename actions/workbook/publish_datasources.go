package workbook

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/paging"
	"github.com/ahillspace/tadx/internal/value"
)

const unmappedDatasourceExplanation = "This can happen when there are naming conflicts on the Tableau Site and should be resolved by using --datasource-binding <source name or content URL>=<target LUID>, where the source name or content URL are the data sources in your workbook and the LUID is the LUID of the published data source on the Tableau Site. Here are the data sources that need mapping along with suggested candidates found by searching the Tableau Site:"

const datasourceCandidateRule = "Candidates match by exact name, or by the workbook's content URL followed by _<digits>. Always check a candidate before remapping."

// matchDatasourceBindings resolves each binding source to exactly one workbook
// datasource content URL. The returned bindings carry that content URL as
// their source so later rewrites and commands are unambiguous.
func matchDatasourceBindings(bindings []DatasourceBinding, datasources []PublishDatasource) ([]DatasourceBinding, error) {
	var matched []DatasourceBinding
	for _, binding := range bindings {
		var urls []string
		for _, datasource := range datasources {
			if (datasource.Caption == binding.Source || datasource.ContentURL == binding.Source) && !slices.Contains(urls, datasource.ContentURL) {
				urls = append(urls, datasource.ContentURL)
			}
		}
		switch len(urls) {
		case 0:
			return nil, datasourceBindingUsage(fmt.Sprintf("--datasource-binding source %q matches no published data source in this workbook; use an exact caption or content URL: %s", binding.Source, workbookDatasourceList(datasources)))
		case 1:
		default:
			return nil, datasourceBindingUsage(fmt.Sprintf("--datasource-binding source %q matches published data sources with content URLs %s; select one by its content URL", binding.Source, strings.Join(urls, ", ")))
		}
		index := slices.IndexFunc(matched, func(prior DatasourceBinding) bool { return prior.Source == urls[0] })
		if index < 0 {
			matched = append(matched, DatasourceBinding{Source: urls[0], TargetLUID: binding.TargetLUID})
		} else if matched[index].TargetLUID != binding.TargetLUID {
			return nil, datasourceBindingUsage(fmt.Sprintf("workbook content URL %q is bound to both %q and %q; bind it once", urls[0], matched[index].TargetLUID, binding.TargetLUID))
		}
	}
	return matched, nil
}

func workbookDatasourceList(datasources []PublishDatasource) string {
	if len(datasources) == 0 {
		return "the workbook has no published data source references"
	}
	names := make([]string, len(datasources))
	for index, datasource := range datasources {
		names[index] = fmt.Sprintf("%q (content URL %s)", datasource.Caption, datasource.ContentURL)
	}
	return strings.Join(names, ", ")
}

func datasourceBindingUsage(message string) error {
	return &errs.Error{ID: "workbook.publish.usage", Kind: errs.KindUsage, Operation: "workbook.publish", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the publish input and review a new preview.", Validation: []errs.ValidationDetail{{Field: "datasource-binding", Code: "invalid", Message: message}}}
}

// readDatasourceBindings reads each exact target datasource on the destination
// site, returning the upload rewrite and how every reference will bind.
func (a *Publisher) readDatasourceBindings(ctx context.Context, input PublishInput, datasources []PublishDatasource, bindings []DatasourceBinding) (map[string]string, []PublishDatasourceBinding, error) {
	var contentURLs map[string]string
	targetLUIDs := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		target, err := a.resolver.ReadDatasource(ctx, binding.TargetLUID)
		if err == nil && target.ContentURL == "" {
			err = errors.New("the datasource has no content URL")
		}
		if err != nil {
			retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Verify the exact target datasource LUID on the destination site.")
			correctiveAction += " Run " + commandhint.Environment(input.Environment, "content", "datasource", "inspect", "--id", binding.TargetLUID) + "."
			return nil, nil, &errs.Error{ID: "workbook.publish.datasource_binding", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: binding.TargetLUID, Environment: input.Environment, Site: input.Site, Summary: fmt.Sprintf("The --datasource-binding target for %q could not be read.", binding.Source), Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
		}
		if contentURLs == nil {
			contentURLs = make(map[string]string, len(bindings))
		}
		contentURLs[binding.Source] = target.ContentURL
		targetLUIDs[binding.Source] = binding.TargetLUID
	}
	var display []PublishDatasourceBinding
	for _, datasource := range datasources {
		row := PublishDatasourceBinding{Datasource: datasource.Caption, ContentURL: datasource.ContentURL, Origin: "workbook", TargetContentURL: datasource.ContentURL}
		if targetLUID, ok := targetLUIDs[datasource.ContentURL]; ok {
			row.Origin, row.TargetContentURL, row.TargetLUID = "explicit", contentURLs[datasource.ContentURL], targetLUID
		}
		display = append(display, row)
	}
	return contentURLs, display, nil
}

// unmappedDatasourceReference is one distinct workbook content URL left to Tableau.
type unmappedDatasourceReference struct {
	caption, contentURL string
}

func unmappedDatasourceReferences(plan PublishPlan) []unmappedDatasourceReference {
	var references []unmappedDatasourceReference
	for _, datasource := range plan.datasources {
		mapped := slices.ContainsFunc(plan.bindings, func(binding DatasourceBinding) bool { return binding.Source == datasource.ContentURL })
		seen := slices.ContainsFunc(references, func(reference unmappedDatasourceReference) bool { return reference.contentURL == datasource.ContentURL })
		if !mapped && !seen {
			references = append(references, unmappedDatasourceReference{caption: datasource.Caption, contentURL: datasource.ContentURL})
		}
	}
	return references
}

// findDatasourceCandidates performs one bounded scan of the destination site.
// A candidate has the reference's exact caption, its exact content URL, or its
// content URL followed by Tableau's _<digits> collision suffix.
func (a *Publisher) findDatasourceCandidates(ctx context.Context, plan PublishPlan, references []unmappedDatasourceReference) ([]PublishDatasourceCandidate, error) {
	items, err := paging.Collect(ctx, a.resolver.ListDatasources, func(item value.Datasource) string { return item.LUID })
	if err != nil {
		return nil, err
	}
	var candidates []PublishDatasourceCandidate
	for _, reference := range references {
		var matches []value.Datasource
		for _, item := range items {
			if item.ContentURL == reference.contentURL || (reference.caption != "" && item.Name == reference.caption) || value.SuffixedContentURL(reference.contentURL, item.ContentURL) {
				matches = append(matches, item)
			}
		}
		slices.SortStableFunc(matches, func(left, right value.Datasource) int {
			if (left.ContentURL == reference.contentURL) != (right.ContentURL == reference.contentURL) {
				if left.ContentURL == reference.contentURL {
					return -1
				}
				return 1
			}
			return strings.Compare(left.ContentURL+"\x00"+left.LUID, right.ContentURL+"\x00"+right.LUID)
		})
		for _, item := range matches[:min(len(matches), publishMaxDatasourceCandidates)] {
			candidates = append(candidates, PublishDatasourceCandidate{
				SourceContentURL: reference.contentURL, Name: item.Name, ContentURL: item.ContentURL, LUID: item.LUID,
				Project: item.ProjectName, OwnerLUID: item.OwnerLUID, UpdatedAt: item.UpdatedAt,
				Command: datasourceRemapCommand(plan, DatasourceBinding{Source: reference.contentURL, TargetLUID: item.LUID}),
			})
		}
	}
	return candidates, nil
}

// datasourceRemapCommand repeats the planned publish with one added binding.
func datasourceRemapCommand(plan PublishPlan, binding DatasourceBinding) string {
	args := append([]string{"content", "workbook", "publish"}, plan.selection...)
	args = append(args, "--project-id", plan.Target.ProjectLUID, "--name", plan.WorkbookName)
	if plan.Overwrite {
		args = append(args, "--overwrite")
	}
	for _, mapped := range append(slices.Clone(plan.bindings), binding) {
		args = append(args, "--datasource-binding", mapped.Source+"="+mapped.TargetLUID)
	}
	return commandhint.Target(plan.Target.Environment, plan.Workspace, args...)
}

func hasExactCandidate(candidates []PublishDatasourceCandidate, contentURL string) bool {
	return slices.ContainsFunc(candidates, func(candidate PublishDatasourceCandidate) bool {
		return candidate.SourceContentURL == contentURL && candidate.ContentURL == contentURL
	})
}

// previewDatasourceCandidates adds remapping suggestions and likely-failure
// warnings for references left to Tableau. It never blocks the preview.
func (a *Publisher) previewDatasourceCandidates(ctx context.Context, plan PublishPlan) PublishPlan {
	references := unmappedDatasourceReferences(plan)
	if len(references) == 0 {
		return plan
	}
	candidates, err := a.findDatasourceCandidates(ctx, plan, references)
	if err != nil {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Data source candidate search was unavailable (%v); references left to Tableau were not checked.", err))
		return plan
	}
	plan.DatasourceCandidates = candidates
	for _, reference := range references {
		if !hasExactCandidate(candidates, reference.contentURL) {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("%s (content URL %s): no data source on this site has that URL; this publish will likely fail.", datasourceLabel(reference), reference.contentURL))
		}
	}
	return plan
}

func datasourceLabel(reference unmappedDatasourceReference) string {
	if reference.caption == "" {
		return reference.contentURL
	}
	return reference.caption
}

// unmappedDatasourceRejection explains Tableau's datasource-not-found
// rejection with exact candidates, or returns nil when the rejection is not
// about references left to Tableau.
func (a *Publisher) unmappedDatasourceRejection(ctx context.Context, plan PublishPlan, cause error) error {
	var code interface{ TableauCode() string }
	references := unmappedDatasourceReferences(plan)
	if !errors.As(cause, &code) || code.TableauCode() != "400011" || len(references) == 0 {
		return nil
	}
	candidates, searchErr := a.findDatasourceCandidates(ctx, plan, references)
	unmapped := references
	if missing := missingDatasourceReferences(references, candidates); searchErr == nil && len(missing) != 0 {
		// When every reference has an exact URL match on the site, Tableau
		// still could not use one of them, so every reference needs review.
		unmapped = missing
	}
	return unmappedDatasourceError(plan, unmapped, candidates, searchErr, cause, "")
}

// ExplainFailedJob explains a failed publish job, which Tableau reports
// without a reason, when the site has no data source at one or more content
// URLs left to Tableau. Without that evidence it returns cause unchanged.
func (a *Publisher) ExplainFailedJob(ctx context.Context, plan PublishPlan, result PublishResult, cause error) error {
	references := unmappedDatasourceReferences(plan)
	if result.Status != "failed" || len(references) == 0 {
		return cause
	}
	candidates, err := a.findDatasourceCandidates(ctx, plan, references)
	missing := missingDatasourceReferences(references, candidates)
	if err != nil || len(missing) == 0 {
		return cause
	}
	return unmappedDatasourceError(plan, missing, candidates, nil, cause, result.JobID)
}

func missingDatasourceReferences(references []unmappedDatasourceReference, candidates []PublishDatasourceCandidate) []unmappedDatasourceReference {
	var missing []unmappedDatasourceReference
	for _, reference := range references {
		if !hasExactCandidate(candidates, reference.contentURL) {
			missing = append(missing, reference)
		}
	}
	return missing
}

// unmappedDatasourceError reports unmapped references with their exact
// candidates. Tableau rejected the publication, so nothing was applied.
func unmappedDatasourceError(plan PublishPlan, unmapped []unmappedDatasourceReference, candidates []PublishDatasourceCandidate, searchErr, cause error, jobID string) error {
	lines := []string{unmappedDatasourceExplanation}
	for _, reference := range unmapped {
		lines = append(lines, fmt.Sprintf("- %s (content URL %s):", datasourceLabel(reference), reference.contentURL))
		if searchErr != nil {
			lines = append(lines, fmt.Sprintf("  Candidate search was unavailable: %v.", searchErr))
			continue
		}
		found := false
		for _, candidate := range candidates {
			if candidate.SourceContentURL != reference.contentURL {
				continue
			}
			found = true
			lines = append(lines, fmt.Sprintf("  - %q, content URL %s, LUID %s, project %q, owner %s, updated %s. Remap: %s", candidate.Name, candidate.ContentURL, candidate.LUID, candidate.Project, candidate.OwnerLUID, candidate.UpdatedAt, candidate.Command))
		}
		if !found {
			lines = append(lines, "  No candidates found. Publish the data source if it does not exist yet. If you already published it, find it with "+commandhint.Environment(plan.Target.Environment, "content", "datasource", "list", "--project-name", "<leaf>")+" or "+commandhint.Environment(plan.Target.Environment, "content", "datasource", "list", "--all")+"; there is no partial-name filter, and --name matches exact names only.")
		}
	}
	lines = append(lines, datasourceCandidateRule)
	return &errs.Error{
		ID: "workbook.publish.datasource_unmapped", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID,
		Environment: plan.Target.Environment, Site: plan.Target.Site,
		Summary: fmt.Sprintf("Your workbook contains %d data source(s) that do not map to a published data source on this Tableau Site.", len(unmapped)),
		Cause:   cause, Retryable: new(false), CorrectiveAction: strings.Join(lines, "\n"),
		TableauJobID: jobID, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeNotApplied,
	}
}
