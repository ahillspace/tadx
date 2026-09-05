package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	adminlist "github.com/ahillspace/tadx/actions/admin/group/list"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	definitionlist "github.com/ahillspace/tadx/actions/pulse/definition/list"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/ahillspace/tadx/internal/catalog"
	resourceadmin "github.com/ahillspace/tadx/internal/resources/admin"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourceflow "github.com/ahillspace/tadx/internal/resources/flow"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

// searchCommands composes the live-first global search action.
type searchCommands struct{ runtime *runtimeDependencies }

func newSearchCommands(runtime *runtimeDependencies) *searchCommands {
	return &searchCommands{runtime: runtime}
}

func (c *searchCommands) Execute(ctx context.Context, input searchaction.Input) (searchaction.Output, error) {
	if input.Catalog {
		_, environment, err := c.runtime.environment(input.Environment, false)
		if err != nil {
			return searchaction.Output{}, capabilitySetupError("search.catalog.setup", "search", input.Environment, input.Site, "Catalog search setup failed.", "Review the selected environment and catalog configuration.", err)
		}
		input.Environment = environment.Alias
		if input.Site == "" {
			input.Site = environment.SiteContentURL
		}
		input.SiteResolved = true
		store := catalog.NewStore(filepath.Dir(c.runtime.configPath), c.runtime.now)
		return searchaction.New(catalogGlobalSearchSource{store: store}).Execute(ctx, input)
	}

	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return searchaction.Output{}, remoteSetupError("search", input.Environment, input.Site, connection.environment, err)
	}
	input.Environment, input.Site, input.SiteResolved = connection.environment.Alias, connection.environment.SiteContentURL, true
	lister, err := newLiveSearchLister(connection)
	if err != nil {
		return searchaction.Output{}, remoteSetupError("search", input.Environment, input.Site, connection.environment, err)
	}
	return searchaction.New(globalSearchSource{adapter: resourcesearch.NewAdapter(lister)}).Execute(ctx, input)
}

type globalSearchSource struct{ adapter *resourcesearch.Adapter }

func (s globalSearchSource) Search(ctx context.Context, input searchaction.Input) (searchaction.Result, error) {
	types, err := searchaction.Types(input.Type)
	if err != nil {
		return searchaction.Result{}, err
	}
	page, err := s.adapter.Search(ctx, resourcesearch.Input{Types: types, Terms: input.Terms, ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return searchaction.Result{}, err
	}
	return searchResult(page, nil), nil
}

type catalogGlobalSearchSource struct {
	store *catalog.Store
}

func (s catalogGlobalSearchSource) Search(ctx context.Context, input searchaction.Input) (searchaction.Result, error) {
	types, err := searchaction.Types(input.Type)
	if err != nil {
		return searchaction.Result{}, err
	}
	statusBefore, statusBeforeErr := s.store.Status(ctx, catalog.Selection{Environment: input.Environment, Site: input.Site, SiteSelected: true})
	if statusBeforeErr != nil && !errors.Is(statusBeforeErr, sql.ErrNoRows) {
		return searchaction.Result{}, statusBeforeErr
	}
	adapter := resourcesearch.NewAdapter(&catalogSearchLister{store: s.store, environment: input.Environment, site: input.Site, skipUnavailable: input.Type == ""})
	page, err := adapter.Search(ctx, resourcesearch.Input{Types: types, Terms: input.Terms, ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return searchaction.Result{}, err
	}
	statusAfter, statusAfterErr := s.store.Status(ctx, catalog.Selection{Environment: input.Environment, Site: input.Site, SiteSelected: true})
	if statusAfterErr != nil && !errors.Is(statusAfterErr, sql.ErrNoRows) {
		return searchaction.Result{}, statusAfterErr
	}
	if statusBeforeErr == nil && statusAfterErr == nil {
		if statusBefore.GenerationID != statusAfter.GenerationID {
			return searchaction.Result{}, invalidCatalogResourceCursor{}
		}
		generation := &searchaction.Generation{ID: statusBefore.GenerationID, Environment: statusBefore.Environment, Site: statusBefore.Site, GeneratedAt: statusBefore.GeneratedAt.UTC().Format(time.RFC3339Nano), Stale: statusBefore.Stale}
		return searchResult(page, generation), nil
	}
	if (statusBeforeErr == nil) != (statusAfterErr == nil) {
		return searchaction.Result{}, invalidCatalogResourceCursor{}
	}
	page.Warnings = append(page.Warnings, "Search used read-through catalog records without a complete catalog generation.")
	return searchResult(page, nil), nil
}

type catalogSearchLister struct {
	store             *catalog.Store
	environment, site string
	skipUnavailable   bool
}

func (s *catalogSearchLister) List(ctx context.Context, resourceType, cursor string, limit int) (resourcesearch.Page, error) {
	state, err := decodeCatalogResourceCursor(cursor)
	if err != nil {
		return resourcesearch.Page{}, err
	}
	result, err := s.store.ReadResources(ctx, catalog.ResourceQuery{Environment: s.environment, Site: s.site, Kind: resourceType, Offset: state.Offset, Limit: limit})
	if err != nil {
		var unavailable interface{ CatalogScopeUnavailable() bool }
		var uninitialized interface{ CatalogUninitialized() bool }
		missing := (errors.As(err, &unavailable) && unavailable.CatalogScopeUnavailable()) || (errors.As(err, &uninitialized) && uninitialized.CatalogUninitialized())
		if missing {
			if s.skipUnavailable {
				return resourcesearch.Page{Warnings: []string{"Catalog does not contain " + resourceType + " resources; that type was omitted."}}, nil
			}
			return resourcesearch.Page{}, catalogSearchScopeUnavailable{resourceType: resourceType, cause: err}
		}
		return resourcesearch.Page{}, err
	}
	fingerprint := catalogResourceFingerprint(result)
	if state.Fingerprint != "" && state.Fingerprint != fingerprint {
		return resourcesearch.Page{}, invalidCatalogResourceCursor{}
	}
	items := make([]resourcesearch.Item, len(result.Entries))
	for index, item := range result.Entries {
		items[index] = resourcesearch.Item{LUID: item.LUID, Type: item.Kind, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner}
	}
	next := ""
	if state.Offset+len(result.Entries) < result.Total {
		next = encodeCatalogResourceCursor(catalogResourceCursor{Offset: state.Offset + len(result.Entries), Fingerprint: fingerprint})
	}
	return resourcesearch.Page{Items: items, NextCursor: next}, nil
}

type catalogSearchScopeUnavailable struct {
	resourceType string
	cause        error
}

func (e catalogSearchScopeUnavailable) Error() string {
	return fmt.Sprintf("catalog does not contain %s resources", e.resourceType)
}
func (e catalogSearchScopeUnavailable) Unwrap() error               { return e.cause }
func (catalogSearchScopeUnavailable) CatalogScopeUnavailable() bool { return true }

type catalogResourceCursor struct {
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
}

type invalidCatalogResourceCursor struct{}

func (invalidCatalogResourceCursor) Error() string {
	return "catalog resource cursor is invalid or stale"
}
func (invalidCatalogResourceCursor) InvalidSearchCursor() bool { return true }

func decodeCatalogResourceCursor(value string) (catalogResourceCursor, error) {
	if value == "" {
		return catalogResourceCursor{}, nil
	}
	if len(value) > 4096 {
		return catalogResourceCursor{}, invalidCatalogResourceCursor{}
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor catalogResourceCursor
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Offset < 1 || cursor.Fingerprint == "" {
		return catalogResourceCursor{}, invalidCatalogResourceCursor{}
	}
	return cursor, nil
}

func encodeCatalogResourceCursor(cursor catalogResourceCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func catalogResourceFingerprint(result catalog.ResourceResult) string {
	data, _ := json.Marshal(struct {
		Total          int    `json:"total"`
		Coverage       string `json:"coverage"`
		GenerationID   string `json:"generation_id"`
		GeneratedAt    string `json:"generated_at"`
		NewestObserved string `json:"newest_observed"`
	}{result.Total, result.Coverage, result.GenerationID, result.GeneratedAt.UTC().Format(time.RFC3339Nano), result.NewestObserved.UTC().Format(time.RFC3339Nano)})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func searchResult(page resourcesearch.Page, generation *searchaction.Generation) searchaction.Result {
	items := make([]searchaction.Item, len(page.Items))
	for index, item := range page.Items {
		items[index] = searchaction.Item{LUID: item.LUID, Type: item.Type, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner, ModifiedAt: item.ModifiedAt}
	}
	return searchaction.Result{Items: items, Page: searchaction.Page{NextCursor: page.NextCursor}, Warnings: page.Warnings, Generation: generation}
}

type liveSearchLister struct {
	environment, site string
	workbooks         workbooklist.Reader
	datasources       datasourcelist.Reader
	flows             flowlist.Reader
	projects          projectlist.Reader
	users             userlist.Reader
	groups            adminlist.Reader
	pulse             *tableaupulse.Client
	definitionPages   map[string]tableaupulse.DefinitionPage
}

func newLiveSearchLister(connection authenticatedTableau) (*liveSearchLister, error) {
	projectClient := tableauproject.NewClient(connection.transport, connection.session, connection.environment.URL)
	projects := resourceproject.NewAdapter(projectClient)
	datasourceClient := tableaudatasource.NewClient(connection.transport, connection.session, connection.environment.URL)
	flowClient := tableauflow.NewClient(connection.transport, connection.session, connection.environment.URL)
	adminClient := tableauadmin.NewClient(connection.transport, connection.session, connection.environment.URL)
	pulseClient, err := tableaupulse.NewClient(connection.transport, connection.session, connection.environment.URL)
	if err != nil {
		return nil, fmt.Errorf("configure authenticated Pulse search client: %w", err)
	}
	return &liveSearchLister{
		environment:     connection.environment.Alias,
		site:            connection.environment.SiteContentURL,
		workbooks:       workbookListReader{adapter: resourceworkbook.NewAdapterWithProjectResolver(tableauworkbook.NewClient(connection.transport, connection.session, connection.environment.URL), projects)},
		datasources:     datasourceListReader{adapter: resourcedatasource.NewAdapterWithProjectResolver(datasourceClient, projects)},
		flows:           flowListReader{adapter: resourceflow.NewAdapter(flowClient, projects)},
		projects:        projectListReader{adapter: projects},
		users:           adminUserListReader{adapter: resourceadmin.NewAdapter(adminClient)},
		groups:          adminGroupListReader{adapter: resourceadmin.NewAdapter(adminClient)},
		pulse:           pulseClient,
		definitionPages: make(map[string]tableaupulse.DefinitionPage),
	}, nil
}

func (s *liveSearchLister) List(ctx context.Context, resourceType, cursor string, limit int) (resourcesearch.Page, error) {
	switch resourceType {
	case "workbook":
		out, err := workbooklist.New(s.workbooks).Execute(ctx, workbooklist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Workbooks))
		for i, item := range out.Workbooks {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "datasource":
		out, err := datasourcelist.New(s.datasources).Execute(ctx, datasourcelist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Datasources))
		for i, item := range out.Datasources {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "flow":
		out, err := flowlist.New(s.flows).Execute(ctx, flowlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Flows))
		for i, item := range out.Flows {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "project":
		out, err := projectlist.New(s.projects).Execute(ctx, projectlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Projects))
		for i, item := range out.Projects {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name, Owner: item.OwnerLUID, ModifiedAt: item.UpdatedAt}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "user":
		out, err := userlist.New(s.users).Execute(ctx, userlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Users))
		for i, item := range out.Users {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "group":
		out, err := adminlist.New(s.groups).Execute(ctx, adminlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Groups))
		for i, item := range out.Groups {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "definition":
		out, err := definitionlist.New(&pulseDefinitionListAdapter{client: s.pulse}).Execute(ctx, definitionlist.Input{Environment: s.environment, Site: s.site, Cursor: cursor, Limit: limit})
		items := make([]resourcesearch.Item, len(out.Definitions))
		for i, item := range out.Definitions {
			items[i] = resourcesearch.Item{LUID: item.LUID, Type: resourceType, Name: item.Name}
		}
		return resourcesearch.Page{Items: items, NextCursor: out.Page.NextCursor}, err
	case "metric":
		return s.listMetrics(ctx, cursor, limit)
	default:
		return resourcesearch.Page{}, fmt.Errorf("unsupported search type %q", resourceType)
	}
}

type metricSearchCursor struct {
	Version             int    `json:"v"`
	DefinitionPageToken string `json:"d,omitempty"`
	DefinitionIndex     int    `json:"i,omitempty"`
	MetricPageToken     string `json:"m,omitempty"`
	DefinitionDigest    string `json:"s,omitempty"`
}

func (s *liveSearchLister) listMetrics(ctx context.Context, encoded string, limit int) (resourcesearch.Page, error) {
	state := metricSearchCursor{Version: 1}
	if encoded != "" {
		data, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.DefinitionIndex < 0 || (state.DefinitionDigest == "" && (state.DefinitionIndex != 0 || state.MetricPageToken != "")) {
			return resourcesearch.Page{}, errors.New("invalid Pulse metric search cursor")
		}
	}
	definitions, ok := s.definitionPages[state.DefinitionPageToken]
	if !ok {
		var err error
		definitions, err = s.pulse.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: 100, PageToken: state.DefinitionPageToken})
		if err != nil {
			return resourcesearch.Page{}, err
		}
		s.definitionPages[state.DefinitionPageToken] = definitions
	}
	digest := definitionPageDigest(definitions)
	if state.DefinitionDigest != "" && state.DefinitionDigest != digest {
		return resourcesearch.Page{}, errors.New("Pulse definition inventory changed during metric search")
	}
	state.DefinitionDigest = digest
	if state.DefinitionIndex > len(definitions.Definitions) {
		return resourcesearch.Page{}, errors.New("invalid Pulse metric search cursor")
	}
	if state.DefinitionIndex == len(definitions.Definitions) {
		if definitions.NextPageToken == "" {
			return resourcesearch.Page{Items: []resourcesearch.Item{}}, nil
		}
		state.DefinitionPageToken = definitions.NextPageToken
		state.DefinitionIndex = 0
		state.DefinitionDigest = ""
		return resourcesearch.Page{Items: []resourcesearch.Item{}, NextCursor: encodeMetricSearchCursor(state)}, nil
	}
	definition := definitions.Definitions[state.DefinitionIndex]
	metrics, err := s.pulse.ListMetrics(ctx, definition.LUID, tableaupulse.PageRequest{PageSize: limit, PageToken: state.MetricPageToken})
	if err != nil {
		return resourcesearch.Page{}, err
	}
	result := resourcesearch.Page{Items: make([]resourcesearch.Item, len(metrics.Metrics))}
	for index, metric := range metrics.Metrics {
		name := metric.Name
		if strings.TrimSpace(name) == "" {
			name = definition.Name
		}
		result.Items[index] = resourcesearch.Item{LUID: metric.LUID, Type: "metric", Name: name}
	}
	if metrics.NextPageToken != "" {
		state.MetricPageToken = metrics.NextPageToken
		result.NextCursor = encodeMetricSearchCursor(state)
		return result, nil
	}
	state.DefinitionIndex++
	state.MetricPageToken = ""
	if state.DefinitionIndex < len(definitions.Definitions) {
		result.NextCursor = encodeMetricSearchCursor(state)
	} else if definitions.NextPageToken != "" {
		state.DefinitionPageToken = definitions.NextPageToken
		state.DefinitionIndex = 0
		state.DefinitionDigest = ""
		result.NextCursor = encodeMetricSearchCursor(state)
	}
	return result, nil
}

func definitionPageDigest(page tableaupulse.DefinitionPage) string {
	identities := make([]string, len(page.Definitions))
	for i, item := range page.Definitions {
		identities[i] = item.LUID
	}
	data, _ := json.Marshal(struct {
		Items []string `json:"items"`
		Next  string   `json:"next"`
	}{identities, page.NextPageToken})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeMetricSearchCursor(cursor metricSearchCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}
