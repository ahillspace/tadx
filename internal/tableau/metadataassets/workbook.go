package metadataassets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

const workbookEmbeddedFields = `id name hasExtracts hasUserReference containsUnsupportedCustomSql extractLastUpdateTime extractLastRefreshTime extractLastIncrementalUpdateTime`
const workbookRelationPage = `{totalCount pageInfo{hasNextPage endCursor} nodes{`
const workbookFieldDescriptionFields = fieldProvenanceFields + ` __typename isHidden folderName ... on DataField {role dataType} ... on AnalyticsField {aggregation} ... on CalculatedField {formula} ... on DatasourceField {remoteField{__typename id name fullyQualifiedName}}`

// WorkbookDatasources observes the workbook and its embedded data sources by exact Metadata API identity.
func (c *Client) WorkbookDatasources(ctx context.Context, luid string) (value.MetadataWorkbookDatasources, error) {
	requests := 0
	out, err := c.workbookDatasourceInventory(ctx, luid, &requests)
	if err != nil {
		return out, err
	}
	for index := range out.Datasources {
		g := &out.Datasources[index]
		if err := c.workbookEmbeddedDetails(ctx, g.Identity.MetadataID, g, &requests, &out.TableauRequestID); err != nil {
			published, identityErr := workbookPublishedGroups(out.Datasources)
			out.Datasources = append(out.Datasources, published...)
			return out, errors.Join(err, identityErr)
		}
		g.Complete = true
	}
	err = c.workbookPublishedDescriptions(ctx, &out, &requests, true)
	return out, err
}

// WorkbookFieldDescriptions observes only field facts and provenance needed for
// workbook schema and audit, without traversing inspection-only relationships.
func (c *Client) WorkbookFieldDescriptions(ctx context.Context, luid string) (value.MetadataWorkbookDatasources, error) {
	requests := 0
	out, err := c.workbookDatasourceInventory(ctx, luid, &requests)
	if err != nil {
		return out, err
	}
	for index := range out.Datasources {
		g := &out.Datasources[index]
		if err := c.workbookEmbeddedDescriptions(ctx, g.Identity.MetadataID, g, &requests, &out.TableauRequestID, false); err != nil {
			published, identityErr := workbookPublishedGroups(out.Datasources)
			out.Datasources = append(out.Datasources, published...)
			return out, errors.Join(err, identityErr)
		}
		g.Complete = true
	}
	err = c.workbookPublishedDescriptions(ctx, &out, &requests, false)
	return out, err
}

func (c *Client) workbookDatasourceInventory(ctx context.Context, luid string, requests *int) (value.MetadataWorkbookDatasources, error) {
	out := value.MetadataWorkbookDatasources{Datasources: []value.MetadataWorkbookDatasource{}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := exact(luid); err != nil {
		return out, err
	}
	cursor := ""
	cursors := map[string]bool{}
	seen := map[string]graphNode{}
	var coverage graphCoverage
	for page := range 100 {
		if *requests >= 400 {
			return out, &ConstraintError{"workbook metadata exceeds 400 request bound"}
		}
		var after any
		if cursor != "" {
			after = cursor
		}
		query := `query WorkbookDatasources($luid:String!,$after:String){workbooks:workbooksConnection(filter:{luid:$luid},first:2,permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{__typename id luid name description ` + graphTagFields + ` items:embeddedDatasourcesConnection(first:100,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS)` + workbookRelationPage + workbookEmbeddedFields + `}}}}}`
		env, r, err := c.graph(ctx, query, map[string]any{"luid": luid, "after": after})
		*requests++
		out.TableauRequestID = r.TableauRequestID
		if err != nil {
			return out, err
		}
		if err = validatePage(env.Data.Workbooks, 2, ""); err != nil {
			return out, protocol("catalog.metadata.read", r, err)
		}
		if len(env.Data.Workbooks.Nodes) != 1 || *env.Data.Workbooks.Total != 1 || *env.Data.Workbooks.Info.More {
			return out, protocol("catalog.metadata.read", r, errors.New("Metadata API did not resolve one exact workbook"))
		}
		wb := env.Data.Workbooks.Nodes[0]
		if wb.ID == "" || wb.Name == "" || wb.LUID != luid {
			return out, protocol("catalog.metadata.read", r, errors.New("workbook identity incomplete or mismatched"))
		}
		if page == 0 {
			out.Identity = identity(wb, "workbook")
			out.Description = wb.Description
			out.Tags, out.TagsObserved = graphTagValues(wb.Tags)
			if !out.TagsObserved {
				out.Warnings = append(out.Warnings, "Workbook tags were not fully observed")
			}
		} else if wb.ID != out.Identity.MetadataID || !reflect.DeepEqual(wb.Description, out.Description) {
			return out, protocol("catalog.metadata.read", r, errors.New("workbook identity or description changed during pagination"))
		}
		p := wb.Items
		if err = validatePage(p, 100, cursor); err != nil {
			return out, protocol("catalog.metadata.read", r, err)
		}
		for _, n := range p.Nodes {
			if n.ID == "" || n.Name == "" {
				return out, protocol("catalog.metadata.read", r, errors.New("embedded datasource identity incomplete"))
			}
			if old, ok := seen[n.ID]; ok {
				if !reflect.DeepEqual(old, n) {
					return out, protocol("catalog.metadata.read", r, errors.New("conflicting embedded datasource identity"))
				}
				continue
			}
			seen[n.ID] = n
			group := value.MetadataWorkbookDatasource{
				Identity: identity(n, "embedded"), Kind: "embedded",
				Fields: []value.FieldDescription{}, Parents: []value.MetadataIdentity{},
				Tables: []value.MetadataTable{}, Databases: []value.MetadataDatabase{},
				Flows: []value.MetadataIdentity{}, Sheets: []value.MetadataIdentity{},
				HasExtracts: n.HasExtracts, HasUserReference: n.HasUserReference,
				ContainsUnsupportedCustomSQL: n.UnsupportedSQL,
				ExtractLastUpdateTime:        n.ExtractUpdated, ExtractLastRefreshTime: n.ExtractRefreshed,
				ExtractLastIncrementalUpdateTime: n.ExtractIncremental,
				Warnings:                         []string{"Tableau Metadata API does not expose descriptions or tags on embedded data sources"},
			}
			out.Datasources = append(out.Datasources, group)
			if len(out.Datasources) > maxItems {
				return out, &ConstraintError{"workbook datasource inventory exceeds item bound"}
			}
		}
		if err = coverage.Page(*p.Total, len(seen), !*p.Info.More); err != nil {
			return out, protocol("catalog.metadata.read", r, err)
		}
		if !*p.Info.More {
			break
		}
		cursor = *p.Info.Cursor
		if cursors[cursor] {
			return out, protocol("catalog.metadata.read", r, errors.New("embedded datasource cursor cycle"))
		}
		cursors[cursor] = true
		if page == 99 {
			return out, &ConstraintError{"workbook datasource inventory exceeds page bound"}
		}
	}
	return out, nil
}

func (c *Client) workbookPublishedDescriptions(ctx context.Context, out *value.MetadataWorkbookDatasources, requests *int, extended bool) error {
	published, err := workbookPublishedGroups(out.Datasources)
	if err != nil {
		return err
	}
	publishedStart := len(out.Datasources)
	out.Datasources = append(out.Datasources, published...)
	complete := true
	var failures []error
	for index := publishedStart; index < len(out.Datasources); index++ {
		target := &out.Datasources[index]
		p := target.Identity
		d, fieldErr := c.datasourceFieldDescriptions(ctx, p.LUID, requests, extended)
		if d.TableauRequestID != "" {
			out.TableauRequestID = d.TableauRequestID
		}
		if d.Identity.MetadataID != "" && (d.Identity.MetadataID != p.MetadataID || d.Identity.Name != p.Name || d.Identity.LUID != p.LUID) {
			failures = append(failures, fmt.Errorf("published datasource %s: metadata identity changed", p.LUID))
			target.Warnings = append(target.Warnings, "Published datasource identity changed during enrichment")
			complete = false
			continue
		}
		target.Fields, target.Description, target.Tags, target.TagsObserved = d.Fields, d.Description, d.Tags, d.TagsObserved
		if fieldErr != nil {
			failures = append(failures, fmt.Errorf("published datasource %s fields: %w", p.LUID, fieldErr))
			target.Warnings = append(target.Warnings, "Published datasource fields are incomplete")
			complete = false
			if workbookRequestWide(fieldErr) {
				out.Complete = false
				return errors.Join(failures...)
			}
		}
		target.Complete = d.Complete && fieldErr == nil
		if extended {
			u, upstreamErr := c.datasourceUpstream(ctx, p.LUID, requests)
			target.Databases, target.Tables = u.Databases, u.Tables
			if u.TableauRequestID != "" {
				out.TableauRequestID = u.TableauRequestID
			}
			if upstreamErr != nil {
				failures = append(failures, fmt.Errorf("published datasource %s upstream: %w", p.LUID, upstreamErr))
				target.Warnings = append(target.Warnings, "Published datasource upstream inventory is incomplete")
				complete = false
				if workbookRequestWide(upstreamErr) {
					out.Complete = false
					return errors.Join(failures...)
				}
			}
			target.Complete = target.Complete && u.Complete && upstreamErr == nil
		}
		complete = complete && target.Complete
	}
	out.Complete = complete
	return errors.Join(failures...)
}

func workbookPublishedGroups(embedded []value.MetadataWorkbookDatasource) ([]value.MetadataWorkbookDatasource, error) {
	parents := map[string]value.MetadataIdentity{}
	parentLUIDs := map[string]string{}
	var groups []value.MetadataWorkbookDatasource
	for _, g := range embedded {
		for _, p := range g.Parents {
			if p.MetadataID == "" || p.LUID == "" || p.Name == "" {
				return nil, errors.New("published parent identity incomplete")
			}
			if previous, ok := parents[p.MetadataID]; ok {
				if !reflect.DeepEqual(previous, p) {
					return nil, errors.New("published parent metadata identity has conflicting LUID or name")
				}
				continue
			}
			if previous, ok := parentLUIDs[p.LUID]; ok && previous != p.MetadataID {
				return nil, errors.New("published parent LUID has conflicting Metadata IDs")
			}
			parents[p.MetadataID] = p
			parentLUIDs[p.LUID] = p.MetadataID
			groups = append(groups, value.MetadataWorkbookDatasource{Identity: p, Kind: "published"})
		}
	}
	return groups, nil
}

func workbookRequestWide(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if limit, ok := errors.AsType[*ConstraintError](err); ok && strings.Contains(limit.Reason, "request bound") {
		return true
	}
	var status interface{ HTTPStatus() int }
	return errors.As(err, &status) && (status.HTTPStatus() == http.StatusUnauthorized || status.HTTPStatus() == http.StatusForbidden)
}

func (c *Client) workbookEmbeddedDetails(ctx context.Context, id string, g *value.MetadataWorkbookDatasource, requests *int, requestID *string) error {
	if err := c.workbookEmbeddedDescriptions(ctx, id, g, requests, requestID, true); err != nil {
		return err
	}
	for _, rel := range []struct{ name, fields string }{
		{"upstreamTablesConnection", tableFields + ` ` + graphTagFields},
		{"upstreamDatabasesConnection", databaseFields + ` ` + graphTagFields},
		{"upstreamFlowsConnection", `__typename id luid name`},
		{"downstreamSheetsConnection", `__typename id luid name`},
	} {
		rows, err := c.workbookRelation(ctx, id, rel.name, rel.fields, requests, requestID, func(nodes []graphNode, r tableau.Response) error {
			for _, n := range nodes {
				switch rel.name {
				case "upstreamTablesConnection":
					g.Tables = append(g.Tables, graphTable(n))
				case "upstreamDatabasesConnection":
					g.Databases = append(g.Databases, graphDatabase(n))
				case "upstreamFlowsConnection":
					g.Flows = append(g.Flows, identity(n, "flow"))
				case "downstreamSheetsConnection":
					g.Sheets = append(g.Sheets, identity(n, "sheet"))
				}
			}
			return nil
		})
		_ = rows
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) workbookEmbeddedDescriptions(ctx context.Context, id string, g *value.MetadataWorkbookDatasource, requests *int, requestID *string, extended bool) error {
	_, err := c.workbookRelation(ctx, id, "parentPublishedDatasourcesConnection", `__typename id luid name`, requests, requestID, func(nodes []graphNode, r tableau.Response) error {
		for _, n := range nodes {
			if n.LUID == "" {
				return protocol("catalog.metadata.read", r, errors.New("published parent LUID missing"))
			}
			g.Parents = append(g.Parents, identity(n, "datasource"))
		}
		return nil
	})
	if err != nil {
		return err
	}
	fields := workbookFieldDescriptionFields
	if extended {
		fields = fieldDescriptionFields
	}
	totalColumns := 0
	_, err = c.workbookRelation(ctx, id, "fieldsConnection", fields, requests, requestID, func(nodes []graphNode, r tableau.Response) error {
		start := len(g.Fields)
		if err := observeFieldPage(nodes, &g.Fields, &totalColumns, extended); err != nil {
			if _, ok := errors.AsType[*ConstraintError](err); ok {
				return err
			}
			return protocol("catalog.metadata.read", r, err)
		}
		return c.enrichFieldPage(ctx, id, nodes, g.Fields[start:], &totalColumns, requests, true, nil, extended)
	})
	return err
}

func (c *Client) workbookRelation(ctx context.Context, id, connection, fields string, requests *int, requestID *string, consume func([]graphNode, tableau.Response) error) ([]graphNode, error) {
	cursor := ""
	cursors := map[string]bool{}
	seen := map[string]graphNode{}
	rows := []graphNode{}
	var coverage graphCoverage
	for page := range 100 {
		if *requests >= 400 {
			return rows, &ConstraintError{"workbook metadata exceeds 400 request bound"}
		}
		var after any
		if cursor != "" {
			after = cursor
		}
		query := `query EmbeddedRelation($id:ID!,$after:String){embedded:embeddedDatasourcesConnection(filter:{idWithin:[$id]},first:2,permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{id name items:` + connection + `(first:100,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS)` + workbookRelationPage + fields + `}}}}}`
		env, r, err := c.graph(ctx, query, map[string]any{"id": id, "after": after})
		*requests++
		*requestID = r.TableauRequestID
		if err != nil {
			return rows, err
		}
		if err = validatePage(env.Data.Embedded, 2, ""); err != nil {
			return rows, protocol("catalog.metadata.read", r, err)
		}
		if len(env.Data.Embedded.Nodes) != 1 || *env.Data.Embedded.Total != 1 || *env.Data.Embedded.Info.More || env.Data.Embedded.Nodes[0].ID != id {
			return rows, protocol("catalog.metadata.read", r, errors.New("embedded datasource identity changed"))
		}
		p := env.Data.Embedded.Nodes[0].Items
		if err = validatePage(p, 100, cursor); err != nil {
			return rows, protocol("catalog.metadata.read", r, err)
		}
		start := len(rows)
		var pageErr error
		for _, n := range p.Nodes {
			if n.ID == "" || n.Name == "" {
				pageErr = protocol("catalog.metadata.read", r, errors.New("related identity incomplete"))
				break
			}
			if old, ok := seen[n.ID]; ok {
				if !reflect.DeepEqual(old, n) {
					pageErr = protocol("catalog.metadata.read", r, errors.New("conflicting related identity"))
					break
				}
				continue
			}
			if len(rows) >= maxItems {
				pageErr = &ConstraintError{fmt.Sprintf("%s exceeds item bound", connection)}
				break
			}
			seen[n.ID] = n
			rows = append(rows, n)
		}
		if err = errors.Join(pageErr, consume(rows[start:], r)); err != nil {
			return rows, err
		}
		if err = coverage.Page(*p.Total, len(seen), !*p.Info.More); err != nil {
			return rows, protocol("catalog.metadata.read", r, err)
		}
		if !*p.Info.More {
			return rows, nil
		}
		cursor = *p.Info.Cursor
		if cursors[cursor] {
			return rows, protocol("catalog.metadata.read", r, errors.New("related cursor cycle"))
		}
		cursors[cursor] = true
		if page == 99 {
			return rows, &ConstraintError{"related inventory exceeds page bound"}
		}
	}
	return rows, &ConstraintError{"related inventory exceeds page bound"}
}
