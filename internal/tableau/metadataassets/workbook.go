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

// WorkbookDatasources observes the workbook and its embedded data sources by exact Metadata API identity.
func (c *Client) WorkbookDatasources(ctx context.Context, luid string) (value.MetadataWorkbookDatasources, error) {
	out := value.MetadataWorkbookDatasources{Datasources: []value.MetadataWorkbookDatasource{}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := exact(luid); err != nil {
		return out, err
	}
	requests := 0
	cursor := ""
	cursors := map[string]bool{}
	seen := map[string]graphNode{}
	var coverage graphCoverage
	for page := range 100 {
		if requests >= 400 {
			return out, &ConstraintError{"workbook metadata exceeds 400 request bound"}
		}
		var after any
		if cursor != "" {
			after = cursor
		}
		query := `query WorkbookDatasources($luid:String!,$after:String){workbooks:workbooksConnection(filter:{luid:$luid},first:2,permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{__typename id luid name description ` + graphTagFields + ` items:embeddedDatasourcesConnection(first:100,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS)` + workbookRelationPage + workbookEmbeddedFields + `}}}}}`
		env, r, err := c.graph(ctx, query, map[string]any{"luid": luid, "after": after})
		requests++
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
			g := &out.Datasources[len(out.Datasources)-1]
			if err = c.workbookEmbeddedDetails(ctx, n.ID, g, &requests, &out.TableauRequestID); err != nil {
				return out, err
			}
			g.Complete = true
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
	parents := map[string]value.MetadataIdentity{}
	parentLUIDs := map[string]string{}
	for _, g := range out.Datasources {
		for _, p := range g.Parents {
			if p.MetadataID == "" || p.LUID == "" || p.Name == "" {
				return out, errors.New("published parent identity incomplete")
			}
			if previous, ok := parents[p.MetadataID]; ok && !reflect.DeepEqual(previous, p) {
				return out, errors.New("published parent metadata identity has conflicting LUID or name")
			}
			if previous, ok := parentLUIDs[p.LUID]; ok && previous != p.MetadataID {
				return out, errors.New("published parent LUID has conflicting Metadata IDs")
			}
			parents[p.MetadataID] = p
			parentLUIDs[p.LUID] = p.MetadataID
		}
	}
	complete := true
	var failures []error
	for _, g := range out.Datasources {
		for _, p := range g.Parents {
			if _, ok := parents[p.MetadataID]; !ok {
				continue
			}
			delete(parents, p.MetadataID)
			published := value.MetadataWorkbookDatasource{Identity: p, Kind: "published"}
			out.Datasources = append(out.Datasources, published)
			target := &out.Datasources[len(out.Datasources)-1]
			d, fieldErr := c.datasourceFieldDescriptions(ctx, p.LUID, &requests, true)
			target.Fields, target.Description, target.Tags, target.TagsObserved = d.Fields, d.Description, d.Tags, d.TagsObserved
			if d.TableauRequestID != "" {
				out.TableauRequestID = d.TableauRequestID
			}
			if d.Identity.MetadataID != "" && (d.Identity.MetadataID != p.MetadataID || d.Identity.Name != p.Name || d.Identity.LUID != p.LUID) {
				failures = append(failures, fmt.Errorf("published datasource %s: metadata identity changed", p.LUID))
				target.Warnings = append(target.Warnings, "Published datasource identity changed during enrichment")
				complete = false
				continue
			}
			if fieldErr != nil {
				failures = append(failures, fmt.Errorf("published datasource %s fields: %w", p.LUID, fieldErr))
				target.Warnings = append(target.Warnings, "Published datasource fields are incomplete")
				complete = false
				if workbookRequestWide(fieldErr) {
					out.Complete = false
					return out, errors.Join(failures...)
				}
			}
			u, upstreamErr := c.datasourceUpstream(ctx, p.LUID, &requests)
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
					return out, errors.Join(failures...)
				}
			}
			target.Complete = d.Complete && u.Complete && fieldErr == nil && upstreamErr == nil
			complete = complete && target.Complete
		}
	}
	out.Complete = complete
	return out, errors.Join(failures...)
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
	for _, rel := range []struct{ name, fields string }{
		{"fieldsConnection", fieldDescriptionFields},
		{"parentPublishedDatasourcesConnection", `__typename id luid name`},
		{"upstreamTablesConnection", tableFields + ` ` + graphTagFields},
		{"upstreamDatabasesConnection", databaseFields + ` ` + graphTagFields},
		{"upstreamFlowsConnection", `__typename id luid name`},
		{"downstreamSheetsConnection", `__typename id luid name`},
	} {
		rows, err := c.workbookRelation(ctx, id, rel.name, rel.fields, requests, requestID, func(n graphNode, r tableau.Response) error {
			switch rel.name {
			case "fieldsConnection":
				if n.FullyQualifiedName == "" {
					return protocol("catalog.metadata.read", r, errors.New("embedded field identity incomplete"))
				}
				v, e := fieldDescription(n)
				if e != nil {
					return protocol("catalog.metadata.read", r, e)
				}
				if e = validatePage(n.Columns, 100, ""); e != nil {
					return protocol("catalog.metadata.read", r, e)
				}
				columns, e := c.fieldColumns(ctx, id, n.ID, n.Columns, requests, true, nil)
				for _, col := range columns {
					if col.ID == "" || col.Name == "" {
						return errors.New("upstream column identity incomplete")
					}
					v.UpstreamColumns = append(v.UpstreamColumns, graphColumn(col))
				}
				g.Fields = append(g.Fields, v)
				if e != nil {
					return e
				}
				return c.fieldRelations(ctx, id, n, &g.Fields[len(g.Fields)-1], requests, true, nil)
			case "parentPublishedDatasourcesConnection":
				if n.LUID == "" {
					return protocol("catalog.metadata.read", r, errors.New("published parent LUID missing"))
				}
				g.Parents = append(g.Parents, identity(n, "datasource"))
			case "upstreamTablesConnection":
				g.Tables = append(g.Tables, graphTable(n))
			case "upstreamDatabasesConnection":
				g.Databases = append(g.Databases, graphDatabase(n))
			case "upstreamFlowsConnection":
				g.Flows = append(g.Flows, identity(n, "flow"))
			case "downstreamSheetsConnection":
				g.Sheets = append(g.Sheets, identity(n, "sheet"))
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

func (c *Client) workbookRelation(ctx context.Context, id, connection, fields string, requests *int, requestID *string, consume func(graphNode, tableau.Response) error) ([]graphNode, error) {
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
		query := `query EmbeddedRelation($id:ID!,$after:String){embedded:embeddedDatasourcesConnection(filter:{id:$id},first:2,permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{id name items:` + connection + `(first:100,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS)` + workbookRelationPage + fields + `}}}}`
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
		for _, n := range p.Nodes {
			if n.ID == "" || n.Name == "" {
				return rows, protocol("catalog.metadata.read", r, errors.New("related identity incomplete"))
			}
			if old, ok := seen[n.ID]; ok {
				if !reflect.DeepEqual(old, n) {
					return rows, protocol("catalog.metadata.read", r, errors.New("conflicting related identity"))
				}
				continue
			}
			seen[n.ID] = n
			rows = append(rows, n)
			if err = consume(n, r); err != nil {
				return rows, err
			}
			if len(rows) > maxItems {
				return rows, &ConstraintError{fmt.Sprintf("%s exceeds item bound", connection)}
			}
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
