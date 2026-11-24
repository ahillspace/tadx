package metadataassets

import (
	"context"
	"errors"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

// DatasourceAuditFields reads one field page with inline relationship pages.
// Other datasource and workbook operations traverse these connections to completion.
func (c *Client) DatasourceAuditFields(ctx context.Context, luid string, includeLineage bool) (value.MetadataDatasourceAudit, error) {
	out := value.MetadataDatasourceAudit{Fields: []value.MetadataAuditField{}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := exact(luid); err != nil {
		return out, err
	}
	projection := fieldSchemaFields
	if includeLineage {
		projection = fieldDescriptionFields
	}
	env, resp, graphErr := c.datasourceGraph(ctx, luid, "fieldsConnection", projection, "")
	out.TableauRequestID = resp.TableauRequestID
	if env.Data == nil {
		return out, graphErr
	}
	items, err := selectPage(env, luid)
	if err != nil {
		return out, protocol("catalog.metadata.read", resp, err)
	}
	parent := env.Data.Parents.Nodes[0]
	if parent.ID == "" || parent.Name == "" {
		return out, protocol("catalog.metadata.read", resp, errors.New("datasource identity incomplete"))
	}
	out.Identity, out.Description = identity(parent, "datasource"), parent.Description
	out.Tags, out.TagsObserved = graphTagValues(parent.Tags)

	nodes, coverage, err := auditPage(items)
	failures := []error{err}
	out.FieldsCoverage = coverage
	for _, n := range nodes {
		if n.FullyQualifiedName == "" {
			failures = append(failures, errors.New("datasource field identity incomplete"))
			continue
		}
		field, err := fieldDescription(n)
		if err != nil {
			// The scalar field identity and values are independent of an
			// incomplete inherited description or remote-field identity.
			field.InheritedObserved = false
			failures = append(failures, err)
		}
		row := value.MetadataAuditField{FieldDescription: field}
		columns, colCoverage, colErr := auditPage(n.Columns)
		row.UpstreamColumnsCoverage = colCoverage
		for _, column := range columns {
			row.UpstreamColumns = append(row.UpstreamColumns, graphColumn(column))
		}
		failures = append(failures, colErr)
		if includeLineage {
			var sheetsErr, refsErr error
			row.Sheets, row.SheetsCoverage, sheetsErr = auditRelated(n.Sheets, "sheet")
			if n.Type == "CalculatedField" {
				row.ReferencedFields, row.ReferencedFieldsCoverage, refsErr = auditRelated(n.References, "field")
			}
			failures = append(failures, sheetsErr, refsErr)
		}
		out.Fields = append(out.Fields, row)
	}
	if out.FieldsCoverage.Omitted != nil {
		out.FieldsCoverage.Omitted = new(*out.FieldsCoverage.TotalCount - len(out.Fields))
	}
	pageErr := errors.Join(failures...)
	if pageErr != nil {
		pageErr = protocol("catalog.metadata.read", resp, pageErr)
	}
	out.Complete = graphErr == nil && pageErr == nil
	return out, errors.Join(graphErr, pageErr)
}

func auditPage(page *graphPage) ([]graphNode, value.MetadataAuditCoverage, error) {
	var coverage value.MetadataAuditCoverage
	problem := validatePage(page, maxPageSize, "")
	if page == nil {
		return nil, coverage, problem
	}
	if page.Info != nil {
		coverage.HasNextPage = page.Info.More
	}
	if page.Total != nil && *page.Total >= len(page.Nodes) {
		coverage.TotalCount = page.Total
	}
	if page.Nodes == nil || len(page.Nodes) > maxPageSize {
		return nil, coverage, problem
	}
	rows, err := distinctFieldRows(page.Nodes)
	if coverage.TotalCount != nil {
		coverage.Omitted = new(*coverage.TotalCount - len(rows))
	}
	if problem == nil {
		var counts graphCoverage
		problem = counts.Page(*page.Total, len(rows), !*page.Info.More)
	}
	return rows, coverage, errors.Join(problem, err)
}

func auditRelated(page *graphPage, kind string) ([]value.MetadataIdentity, *value.MetadataAuditCoverage, error) {
	rows, coverage, err := auditPage(page)
	var identities []value.MetadataIdentity
	for _, row := range rows {
		identities = append(identities, identity(row, kind))
	}
	return identities, &coverage, err
}
