package workbook

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

const unmappedDatasourceExplanation = "This can happen when there are naming conflicts on the Tableau Site and should be resolved by using --datasource-binding <source name or content URL>=<target LUID>, where the source name or content URL are the data sources in your workbook and the LUID is the LUID of the published data source on the Tableau Site. Here are the data sources that need mapping along with suggested candidates found by searching the Tableau Site:"

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

// unmappedDatasourceReferences returns the first workbook reference to each
// distinct content URL left to Tableau.
func unmappedDatasourceReferences(plan PublishPlan) []PublishDatasource {
	var references []PublishDatasource
	for _, datasource := range plan.datasources {
		mapped := slices.ContainsFunc(plan.bindings, func(binding DatasourceBinding) bool { return binding.Source == datasource.ContentURL })
		seen := slices.ContainsFunc(references, func(reference PublishDatasource) bool { return reference.ContentURL == datasource.ContentURL })
		if !mapped && !seen {
			references = append(references, datasource)
		}
	}
	return references
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

func datasourceLabel(reference PublishDatasource) string {
	if reference.Caption == "" {
		return reference.ContentURL
	}
	return reference.Caption
}

// searchableByName reports whether a caption can be sent in a Tableau name
// filter, which cannot carry a comma.
func searchableByName(caption string) bool {
	return caption != "" && !strings.Contains(caption, ",")
}

// unmappedDatasourceRejection explains Tableau's datasource-not-found
// rejection. One exact content URL lookup finds the references left to
// Tableau that have no datasource on the site, and one exact name lookup
// finds candidates for them. It returns no error when the rejection is not
// about missing references, with a note for Tableau's own error when that
// could not be checked.
func (a *Publisher) unmappedDatasourceRejection(ctx context.Context, plan PublishPlan, cause error) (string, error) {
	var code interface{ TableauCode() string }
	references := unmappedDatasourceReferences(plan)
	if !errors.As(cause, &code) || code.TableauCode() != "400011" || len(references) == 0 {
		return "", nil
	}
	contentURLs := make([]string, len(references))
	for index, reference := range references {
		contentURLs[index] = reference.ContentURL
	}
	present, err := a.resolver.FindDatasourcesByContentURL(ctx, contentURLs)
	if err != nil {
		return fmt.Sprintf(" The workbook's data sources could not be checked on this site: %v.", err), nil
	}
	var missing []PublishDatasource
	var names []string
	for _, reference := range references {
		if slices.ContainsFunc(present, func(item value.Datasource) bool { return item.ContentURL == reference.ContentURL }) {
			continue
		}
		missing = append(missing, reference)
		if searchableByName(reference.Caption) {
			names = append(names, reference.Caption)
		}
	}
	if len(missing) == 0 {
		return "", nil
	}
	var candidates []value.Datasource
	var searchErr error
	if len(names) != 0 {
		candidates, searchErr = a.resolver.FindDatasourcesByName(ctx, names)
	}
	return "", unmappedDatasourceError(plan, missing, candidates, searchErr, cause)
}

// unmappedDatasourceError reports each missing reference with the
// datasources that have its exact name. Tableau rejected the publication, so
// nothing was applied.
func unmappedDatasourceError(plan PublishPlan, missing []PublishDatasource, candidates []value.Datasource, searchErr, cause error) error {
	lines := []string{unmappedDatasourceExplanation}
	for _, reference := range missing {
		lines = append(lines, fmt.Sprintf("- %s (content URL %s):", datasourceLabel(reference), reference.ContentURL))
		if strings.Contains(reference.Caption, ",") {
			lines = append(lines, "  Not searched by name because Tableau name filters cannot contain a comma.")
			continue
		}
		if searchErr != nil && reference.Caption != "" {
			lines = append(lines, fmt.Sprintf("  Candidate search was unavailable: %v.", searchErr))
			continue
		}
		var matches []value.Datasource
		for _, candidate := range candidates {
			if reference.Caption != "" && strings.EqualFold(candidate.Name, reference.Caption) {
				matches = append(matches, candidate)
			}
		}
		slices.SortFunc(matches, func(left, right value.Datasource) int {
			return strings.Compare(left.ContentURL+"\x00"+left.LUID, right.ContentURL+"\x00"+right.LUID)
		})
		for _, match := range matches[:min(len(matches), publishMaxDatasourceCandidates)] {
			command := datasourceRemapCommand(plan, DatasourceBinding{Source: reference.ContentURL, TargetLUID: match.LUID})
			lines = append(lines, fmt.Sprintf("  - %q, content URL %s, LUID %s, project %q (LUID %s), owner %s, updated %s. Remap: %s", match.Name, match.ContentURL, match.LUID, match.ProjectName, match.ProjectLUID, match.OwnerLUID, match.UpdatedAt, command))
		}
		if omitted := len(matches) - publishMaxDatasourceCandidates; omitted > 0 {
			lines = append(lines, fmt.Sprintf("  %d more data source(s) have this name; list them with %s.", omitted, commandhint.Environment(plan.Target.Environment, "content", "datasource", "list", "--name", reference.Caption)))
		}
		if len(matches) == 0 {
			lines = append(lines, "  No candidates found. Publish the data source if it does not exist yet. If you already published it, find it with "+commandhint.Environment(plan.Target.Environment, "content", "datasource", "list", "--project-name", "<leaf>")+" or "+commandhint.Environment(plan.Target.Environment, "content", "datasource", "list", "--all")+"; there is no partial-name filter, and --name matches exact names only.")
		}
	}
	lines = append(lines, "Always check a candidate before remapping.")
	return &errs.Error{
		ID: "workbook.publish.datasource_unmapped", Kind: errs.KindOperation, Operation: "workbook.publish", Resource: plan.Target.ExistingLUID,
		Environment: plan.Target.Environment, Site: plan.Target.Site,
		Summary: fmt.Sprintf("Your workbook contains %d data source(s) that do not map to a published data source on this Tableau Site.", len(missing)),
		Cause:   cause, Retryable: new(false), CorrectiveAction: strings.Join(lines, "\n"),
		TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeNotApplied,
	}
}
