package metadataassets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ahillspace/tadx/internal/tableau"
	"github.com/ahillspace/tadx/internal/value"
)

type graphPage struct {
	Nodes []graphNode `json:"nodes"`
	Total *int        `json:"totalCount"`
	Info  *graphInfo  `json:"pageInfo"`
}
type graphInfo struct {
	More   *bool   `json:"hasNextPage"`
	Cursor *string `json:"endCursor"`
}
type graphNode struct {
	ID                 string           `json:"id"`
	LUID               string           `json:"luid"`
	Name               string           `json:"name"`
	Type               string           `json:"__typename"`
	Description        *string          `json:"description"`
	ConnectionType     string           `json:"connectionType"`
	Embedded           bool             `json:"isEmbedded"`
	FilePath           string           `json:"filePath"`
	FullName           string           `json:"fullName"`
	Schema             string           `json:"schema"`
	RemoteType         string           `json:"remoteType"`
	Nullable           *bool            `json:"isNullable"`
	Database           *graphIdentity   `json:"database"`
	Table              *graphIdentity   `json:"table"`
	Contact            *graphIdentity   `json:"contact"`
	Tags               *graphTags       `json:"tagsConnection"`
	Items              *graphPage       `json:"items"`
	FullyQualifiedName string           `json:"fullyQualifiedName"`
	Inherited          []graphInherited `json:"descriptionInherited"`
	Columns            *graphPage       `json:"upstreamColumnsConnection"`
}
type graphIdentity struct {
	ID   string `json:"id"`
	LUID string `json:"luid"`
	Name string `json:"name"`
	Type string `json:"__typename"`
}
type graphTags struct {
	Nodes []struct {
		Name string `json:"name"`
	} `json:"nodes"`
	Total *int       `json:"totalCount"`
	Info  *graphInfo `json:"pageInfo"`
}
type graphInherited struct {
	Value     *string  `json:"value"`
	ID        string   `json:"assetId"`
	Attribute string   `json:"attribute"`
	Distance  *int     `json:"distance"`
	Edges     []string `json:"edges"`
}
type graphEnvelope struct {
	Data *struct {
		Items   *graphPage `json:"items"`
		Parents *struct {
			Nodes []graphNode `json:"nodes"`
		} `json:"parents"`
	} `json:"data"`
	Errors   json.RawMessage `json:"errors"`
	Warnings json.RawMessage `json:"warnings"`
}

const graphTagFields = `tagsConnection(first:100,orderBy:{field:NAME,direction:ASC}){totalCount pageInfo{hasNextPage endCursor} nodes{name}}`
const databaseFields = `__typename id luid name description connectionType isEmbedded ... on File {filePath}`
const tableFields = `__typename id luid name description fullName schema database{__typename id luid name}`
const columnFields = `__typename id luid name description remoteType isNullable table{__typename id name ... on DatabaseTable {luid}}`

func (c *Client) graph(ctx context.Context, query string, variables map[string]any) (graphEnvelope, tableau.Response, error) {
	if e := c.configured(); e != nil {
		return graphEnvelope{}, tableau.Response{}, e
	}
	body, e := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{query, variables})
	if e != nil {
		return graphEnvelope{}, tableau.Response{}, e
	}
	r, e := c.transport.Do(ctx, c.session, tableau.Request{Method: http.MethodPost, ServerURL: c.serverURL, Path: "/api/metadata/graphql", Body: body, ContentType: "application/json", Accept: "application/json", Operation: "catalog.metadata.read", MaxResponseBytes: maxResponseBytes})
	if e != nil {
		return graphEnvelope{}, r, e
	}
	var out graphEnvelope
	if e = json.Unmarshal(r.Body, &out); e != nil {
		return out, r, protocol("catalog.metadata.read", r, e)
	}
	for _, issues := range []json.RawMessage{out.Errors, out.Warnings} {
		s := strings.TrimSpace(string(issues))
		if s != "" && s != "null" && s != "[]" {
			return out, r, protocol("catalog.metadata.read", r, errors.New("Metadata API returned errors or warnings; requested coverage is not established"))
		}
	}
	if out.Data == nil {
		return out, r, protocol("catalog.metadata.read", r, errors.New("Metadata API omitted data"))
	}
	return out, r, nil
}
func normalizeQuery(q Query) (Query, map[string]any, error) {
	if q.Limit == 0 {
		q.Limit = 25
	}
	if q.Limit < 1 || q.Limit > maxPageSize {
		return q, nil, errors.New("metadata page limit must be between 1 and 100")
	}
	if len(q.Cursor) > 4096 {
		return q, nil, errors.New("metadata cursor exceeds bound")
	}
	filter := map[string]any{}
	for k, s := range map[string]string{"luid": q.LUID, "id": q.MetadataID, "name": q.Name, "text": q.Text} {
		if s != "" {
			if e := exact(s); e != nil {
				return q, nil, e
			}
			filter[k] = s
		}
	}
	if q.LUID != "" && q.MetadataID != "" {
		return q, nil, errors.New("select either REST LUID or Metadata ID")
	}
	var after any
	if q.Cursor != "" {
		after = q.Cursor
	}
	return q, map[string]any{"filter": filter, "first": q.Limit, "after": after}, nil
}
func validatePage(p *graphPage, limit int, cursor string) error {
	if p == nil || p.Nodes == nil || p.Total == nil || p.Info == nil || p.Info.More == nil {
		return errors.New("Metadata API omitted collection coverage")
	}
	if len(p.Nodes) > limit || *p.Total < len(p.Nodes) || *p.Total < 0 {
		return errors.New("Metadata API returned inconsistent page bounds")
	}
	if cursor == "" && !*p.Info.More && *p.Total != len(p.Nodes) {
		return errors.New("Metadata API terminal page does not cover its reported count")
	}
	if *p.Info.More && (len(p.Nodes) == 0 || p.Info.Cursor == nil || *p.Info.Cursor == "" || *p.Info.Cursor == cursor) {
		return errors.New("Metadata API pagination did not advance")
	}
	return nil
}
func selectPage(env graphEnvelope, parent string) (*graphPage, error) {
	if parent == "" {
		return env.Data.Items, nil
	}
	if env.Data.Parents == nil || len(env.Data.Parents.Nodes) != 1 || env.Data.Parents.Nodes[0].LUID != parent {
		return nil, errors.New("Metadata API did not resolve the exact parent identity")
	}
	return env.Data.Parents.Nodes[0].Items, nil
}
func (c *Client) discover(ctx context.Context, q Query, kind string) (*graphPage, tableau.Response, error) {
	q, vars, e := normalizeQuery(q)
	if e != nil {
		return nil, tableau.Response{}, e
	}
	var connection, filterType, fields string
	switch kind {
	case "database":
		connection, filterType, fields = "databasesConnection", "Database_Filter", databaseFields
	case "table":
		connection, filterType, fields = "databaseTablesConnection", "DatabaseTable_Filter", tableFields
	case "column":
		connection, filterType, fields = "columnsConnection", "Column_Filter", columnFields
		if q.Text != "" {
			return nil, tableau.Response{}, &ConstraintError{"column search has no upstream text filter; use bounded table scope"}
		}
	default:
		return nil, tableau.Response{}, errors.New("invalid discovery type")
	}
	args := `(filter:$filter,first:$first,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS)`
	selection := `{totalCount pageInfo{hasNextPage endCursor} nodes{` + fields + " " + graphTagFields + `}}`
	decl := `$filter:` + filterType + `,$first:Int!,$after:String`
	body := `items:` + connection + args + selection
	if q.ParentLUID != "" {
		if e := exact(q.ParentLUID); e != nil {
			return nil, tableau.Response{}, e
		}
		parentConnection, nested := "databasesConnection", "tablesConnection"
		if kind == "column" {
			parentConnection, nested = "databaseTablesConnection", "columnsConnection"
		} else if kind != "table" {
			return nil, tableau.Response{}, errors.New("database discovery has no parent selector")
		}
		decl += `,$parent:String!`
		vars["parent"] = q.ParentLUID
		body = `parents:` + parentConnection + `(filter:{luid:$parent},first:2,permissionMode:OBFUSCATE_RESULTS){nodes{luid items:` + nested + args + selection + `}}`
	}
	env, r, e := c.graph(ctx, `query CatalogPage(`+decl+`){`+body+`}`, vars)
	if e != nil {
		return nil, r, e
	}
	p, e := selectPage(env, q.ParentLUID)
	if e == nil {
		e = validatePage(p, q.Limit, q.Cursor)
	}
	if e != nil {
		return nil, r, protocol("catalog.metadata.read", r, e)
	}
	for _, n := range p.Nodes {
		if n.ID == "" || n.Name == "" {
			return nil, r, protocol("catalog.metadata.read", r, errors.New("Metadata API returned an incomplete or obfuscated identity"))
		}
		if (q.LUID != "" && n.LUID != q.LUID) || (q.MetadataID != "" && n.ID != q.MetadataID) || (q.Name != "" && n.Name != q.Name) {
			return nil, r, protocol("catalog.metadata.read", r, errors.New("Metadata API returned a mismatched exact selector"))
		}
	}
	return p, r, nil
}
func identity(n graphNode, kind string) value.MetadataIdentity {
	t := n.Type
	if t == "" {
		t = kind
	}
	return value.MetadataIdentity{MetadataID: n.ID, LUID: n.LUID, Name: n.Name, Type: t}
}
func related(n *graphIdentity, kind string) value.MetadataIdentity {
	if n == nil {
		return value.MetadataIdentity{Type: kind}
	}
	t := n.Type
	if t == "" {
		t = kind
	}
	return value.MetadataIdentity{MetadataID: n.ID, LUID: n.LUID, Name: n.Name, Type: t}
}
func graphTagValues(p *graphTags) ([]string, bool) {
	if p == nil || p.Nodes == nil || p.Total == nil || p.Info == nil || p.Info.More == nil || *p.Info.More || *p.Total != len(p.Nodes) {
		return nil, false
	}
	out := make([]string, 0, len(p.Nodes))
	for _, n := range p.Nodes {
		if strings.TrimSpace(n.Name) == "" {
			return nil, false
		}
		out = append(out, n.Name)
	}
	sort.Strings(out)
	return out, true
}
func graphDatabase(n graphNode) value.MetadataDatabase {
	ts, seen := graphTagValues(n.Tags)
	return value.MetadataDatabase{MetadataIdentity: identity(n, "database"), Description: n.Description, ConnectionType: n.ConnectionType, Embedded: n.Embedded, FilePath: n.FilePath, Tags: ts, TagsObserved: seen}
}
func graphTable(n graphNode) value.MetadataTable {
	ts, seen := graphTagValues(n.Tags)
	return value.MetadataTable{MetadataIdentity: identity(n, "table"), Description: n.Description, Database: related(n.Database, "database"), FullName: n.FullName, Schema: n.Schema, Tags: ts, TagsObserved: seen}
}
func graphColumn(n graphNode) value.MetadataColumn {
	ts, seen := graphTagValues(n.Tags)
	return value.MetadataColumn{MetadataIdentity: identity(n, "column"), Description: n.Description, Table: related(n.Table, "table"), RemoteType: n.RemoteType, Nullable: n.Nullable, Tags: ts, TagsObserved: seen}
}
func mapped[T any](p *graphPage, r tableau.Response, convert func(graphNode) T) (Page[T], error) {
	items := make([]T, 0, len(p.Nodes))
	seen := map[string]graphNode{}
	seenLUID := map[string]string{}
	for _, n := range p.Nodes {
		if n.LUID != "" {
			if previous, ok := seenLUID[n.LUID]; ok && previous != n.ID {
				return Page[T]{}, protocol("catalog.metadata.read", r, errors.New("multiple Metadata IDs claim one REST identity"))
			}
			seenLUID[n.LUID] = n.ID
		}
		if old, ok := seen[n.ID]; ok {
			if !reflect.DeepEqual(old, n) {
				return Page[T]{}, protocol("catalog.metadata.read", r, errors.New("conflicting duplicate metadata identity"))
			}
			continue
		}
		seen[n.ID] = n
		items = append(items, convert(n))
	}
	cursor := ""
	if *p.Info.More {
		cursor = *p.Info.Cursor
	}
	return Page[T]{Items: items, NextCursor: cursor, Total: *p.Total, Complete: !*p.Info.More, ObservedAt: time.Now().UTC().Format(time.RFC3339), TableauRequestID: r.TableauRequestID}, nil
}
func (c *Client) DiscoverDatabases(ctx context.Context, q Query) (Page[value.MetadataDatabase], error) {
	p, r, e := c.discover(ctx, q, "database")
	if e != nil {
		return Page[value.MetadataDatabase]{}, e
	}
	return mapped(p, r, graphDatabase)
}
func (c *Client) DiscoverTables(ctx context.Context, q Query) (Page[value.MetadataTable], error) {
	p, r, e := c.discover(ctx, q, "table")
	if e != nil {
		return Page[value.MetadataTable]{}, e
	}
	return mapped(p, r, graphTable)
}
func (c *Client) DiscoverColumns(ctx context.Context, q Query) (Page[value.MetadataColumn], error) {
	p, r, e := c.discover(ctx, q, "column")
	if e != nil {
		return Page[value.MetadataColumn]{}, e
	}
	return mapped(p, r, graphColumn)
}

func (c *Client) datasourcePage(ctx context.Context, luid, connection, fields, cursor string) (*graphPage, graphNode, tableau.Response, error) {
	parentFields := "luid"
	if connection == "fieldsConnection" {
		parentFields = "id luid name description " + graphTagFields
	}
	query := `query DatasourceUpstream($parent:String!,$first:Int!,$after:String){parents:publishedDatasourcesConnection(filter:{luid:$parent},first:2,permissionMode:OBFUSCATE_RESULTS){nodes{` + parentFields + ` items:` + connection + `(first:$first,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{` + fields + `}}}}}`
	var after any
	if cursor != "" {
		after = cursor
	}
	env, r, e := c.graph(ctx, query, map[string]any{"parent": luid, "first": 100, "after": after})
	if e != nil {
		return nil, graphNode{}, r, e
	}
	p, e := selectPage(env, luid)
	if e == nil {
		e = validatePage(p, 100, cursor)
	}
	if e != nil {
		return nil, graphNode{}, r, protocol("catalog.metadata.read", r, e)
	}
	return p, env.Data.Parents.Nodes[0], r, nil
}
func (c *Client) DatasourceUpstream(ctx context.Context, luid string) (DatasourceUpstream, error) {
	out := DatasourceUpstream{LUID: luid, Databases: []value.MetadataDatabase{}, Tables: []value.MetadataTable{}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if e := exact(luid); e != nil {
		return out, e
	}
	for _, kind := range []string{"database", "table"} {
		connection, fields := "upstreamDatabasesConnection", databaseFields+" "+graphTagFields
		if kind == "table" {
			connection, fields = "upstreamTablesConnection", tableFields+" "+graphTagFields
		}
		cursor := ""
		seenCursor := map[string]bool{}
		seen := map[string]graphNode{}
		for page := 0; page < 100; page++ {
			p, _, r, e := c.datasourcePage(ctx, luid, connection, fields, cursor)
			out.TableauRequestID = r.TableauRequestID
			if e != nil {
				return out, e
			}
			for _, n := range p.Nodes {
				if n.ID == "" || n.Name == "" {
					return out, protocol("catalog.metadata.read", r, errors.New("upstream identity incomplete or obfuscated"))
				}
				if old, ok := seen[n.ID]; ok {
					if !reflect.DeepEqual(old, n) {
						return out, protocol("catalog.metadata.read", r, errors.New("conflicting upstream identity"))
					}
					continue
				}
				seen[n.ID] = n
				if kind == "database" {
					out.Databases = append(out.Databases, graphDatabase(n))
				} else {
					out.Tables = append(out.Tables, graphTable(n))
				}
				if len(out.Databases)+len(out.Tables) > maxItems {
					return out, &ConstraintError{"upstream inventory exceeds 10000 item bound"}
				}
			}
			if !*p.Info.More {
				break
			}
			cursor = *p.Info.Cursor
			if seenCursor[cursor] {
				return out, errors.New("upstream cursor cycle")
			}
			seenCursor[cursor] = true
			if page == 99 {
				return out, &ConstraintError{"upstream inventory exceeds page bound"}
			}
		}
	}
	out.Complete = true
	return out, nil
}

func (c *Client) DatasourceFieldDescriptions(ctx context.Context, luid string) (DatasourceDescriptions, error) {
	out := DatasourceDescriptions{LUID: luid, Fields: []value.FieldDescription{}, ObservedAt: time.Now().UTC().Format(time.RFC3339)}
	if e := exact(luid); e != nil {
		return out, e
	}
	const fields = `id name fullyQualifiedName description descriptionInherited(inheritanceType:FIRST,permissionMode:OBFUSCATE_RESULTS){value assetId attribute distance edges} upstreamColumnsConnection(first:100,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{` + columnFields + ` ` + graphTagFields + `}}`
	cursor := ""
	seenCursor := map[string]bool{}
	seen := map[string]graphNode{}
	totalColumns := 0
	requests := 0
	for page := 0; page < 100; page++ {
		if requests >= 200 {
			return out, &ConstraintError{"datasource provenance exceeds 200 request bound"}
		}
		p, parent, r, e := c.datasourcePage(ctx, luid, "fieldsConnection", fields, cursor)
		requests++
		out.TableauRequestID = r.TableauRequestID
		if e != nil {
			return out, e
		}
		if parent.ID == "" || parent.Name == "" {
			return out, protocol("catalog.metadata.read", r, errors.New("datasource identity incomplete"))
		}
		if page == 0 {
			out.Identity = identity(parent, "datasource")
			out.Description = parent.Description
			out.Tags, out.TagsObserved = graphTagValues(parent.Tags)
		}
		for _, n := range p.Nodes {
			if n.ID == "" || n.Name == "" || n.FullyQualifiedName == "" {
				return out, protocol("catalog.metadata.read", r, errors.New("datasource field identity incomplete"))
			}
			if old, ok := seen[n.ID]; ok {
				if !reflect.DeepEqual(old, n) {
					return out, protocol("catalog.metadata.read", r, errors.New("conflicting field identity"))
				}
				continue
			}
			seen[n.ID] = n
			v := value.FieldDescription{MetadataID: n.ID, Name: n.Name, FullyQualifiedName: n.FullyQualifiedName, Description: n.Description, Inherited: []value.DescriptionObservation{}, InheritedObserved: n.Inherited != nil, UpstreamColumns: []value.MetadataColumn{}}
			if len(n.Inherited) > 100 {
				return out, &ConstraintError{"inherited descriptions exceed bound"}
			}
			for _, a := range n.Inherited {
				if a.ID == "" || a.Attribute == "" {
					return out, protocol("catalog.metadata.read", r, errors.New("inherited description lacks provenance"))
				}
				v.Inherited = append(v.Inherited, value.DescriptionObservation{Value: a.Value, SourceMetadataID: a.ID, Attribute: a.Attribute, Distance: a.Distance, Edges: a.Edges})
			}
			if e = validatePage(n.Columns, 100, ""); e != nil {
				return out, protocol("catalog.metadata.read", r, e)
			}
			columns, e := c.fieldColumns(ctx, luid, n.ID, n.Columns, &requests)
			if e != nil {
				return out, e
			}
			for _, col := range columns {
				if col.ID == "" || col.Name == "" {
					return out, protocol("catalog.metadata.read", r, errors.New("upstream column identity incomplete"))
				}
				v.UpstreamColumns = append(v.UpstreamColumns, graphColumn(col))
				totalColumns++
			}
			if totalColumns > maxItems {
				return out, &ConstraintError{"field provenance exceeds 10000 column bound"}
			}
			out.Fields = append(out.Fields, v)
			if len(out.Fields) > maxItems {
				return out, &ConstraintError{"datasource fields exceed bound"}
			}
		}
		if !*p.Info.More {
			out.Complete = true
			return out, nil
		}
		cursor = *p.Info.Cursor
		if seenCursor[cursor] {
			return out, errors.New("field cursor cycle")
		}
		seenCursor[cursor] = true
	}
	return out, &ConstraintError{fmt.Sprintf("datasource fields exceed %d pages", 100)}
}

// fieldColumns pages one field relationship independently from the outer field page.
func (c *Client) fieldColumns(ctx context.Context, datasource, field string, first *graphPage, requests *int) ([]graphNode, error) {
	out := []graphNode{}
	seen := map[string]graphNode{}
	cursors := map[string]bool{}
	p := first
	for {
		for _, n := range p.Nodes {
			if old, ok := seen[n.ID]; ok {
				if !reflect.DeepEqual(old, n) {
					return nil, errors.New("conflicting upstream column identity")
				}
				continue
			}
			seen[n.ID] = n
			out = append(out, n)
			if len(out) > maxItems {
				return nil, &ConstraintError{"field column count exceeds bound"}
			}
		}
		if !*p.Info.More {
			return out, nil
		}
		cursor := *p.Info.Cursor
		if cursors[cursor] {
			return nil, errors.New("field column cursor cycle")
		}
		cursors[cursor] = true
		if *requests >= 200 {
			return nil, &ConstraintError{"datasource provenance exceeds 200 request bound"}
		}
		*requests++
		query := `query FieldColumns($parent:String!,$field:ID!,$after:String){parents:publishedDatasourcesConnection(filter:{luid:$parent},first:2,permissionMode:OBFUSCATE_RESULTS){nodes{luid items:fieldsConnection(filter:{id:$field},first:2,permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{id upstreamColumnsConnection(first:100,after:$after,orderBy:{field:ID,direction:ASC},permissionMode:OBFUSCATE_RESULTS){totalCount pageInfo{hasNextPage endCursor} nodes{` + columnFields + ` ` + graphTagFields + `}}}}}}}`
		env, r, e := c.graph(ctx, query, map[string]any{"parent": datasource, "field": field, "after": cursor})
		if e != nil {
			return nil, e
		}
		fields, e := selectPage(env, datasource)
		if e == nil {
			e = validatePage(fields, 2, "")
		}
		if e != nil {
			return nil, protocol("catalog.metadata.read", r, e)
		}
		if len(fields.Nodes) != 1 || fields.Nodes[0].ID != field || *fields.Info.More || *fields.Total != 1 {
			return nil, protocol("catalog.metadata.read", r, errors.New("field identity changed while paging columns"))
		}
		p = fields.Nodes[0].Columns
		if e = validatePage(p, 100, cursor); e != nil {
			return nil, protocol("catalog.metadata.read", r, e)
		}
	}
}
