package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	catalogstatus "github.com/ahillspace/tadx/actions/catalog/status"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
)

const catalogSourceName = "tableau-rest"

type catalogRunner interface {
	Run(context.Context, tableaucatalog.RunRequest, tableaucatalog.BatchWriter) (tableaucatalog.Result, error)
}

type catalogExecutorFactory func(context.Context, string, string) (tableaucatalog.Executor, error)
type catalogRunnerFactory func(tableaucatalog.Executor) (catalogRunner, error)

type catalogHydrator struct {
	store       *corecatalog.Store
	now         func() time.Time
	executorFor catalogExecutorFactory
	newRunner   catalogRunnerFactory
}

func (h catalogHydrator) Hydrate(ctx context.Context, input catalogrefresh.HydrationRequest) (catalogrefresh.HydrationResult, error) {
	if h.store == nil || h.executorFor == nil || h.newRunner == nil {
		return catalogrefresh.HydrationResult{}, errors.New("catalog hydrator is not configured")
	}
	requested, err := catalogScopes(input.RequestedScopes)
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	plan, err := tableaucatalog.PlanScopes(requested)
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	if !slices.Equal(scopeStrings(plan.Requested), input.RequestedScopes) || !slices.Equal(scopeStrings(plan.Implicit), input.ImplicitScopes) {
		return catalogrefresh.HydrationResult{}, errors.New("catalog action and collector scope plans differ")
	}
	executor, err := h.executorFor(ctx, input.Environment, input.Site)
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	runner, err := h.newRunner(executor)
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	now := h.now
	if now == nil {
		now = time.Now
	}
	generatedAt := now().UTC()
	writer, err := h.store.BeginRefreshGeneration(ctx, corecatalog.GenerationMetadata{
		Environment: input.Environment, Site: input.Site, GeneratedAt: generatedAt, Source: catalogSourceName,
		RequestedScopes: append([]string(nil), input.RequestedScopes...), ImplicitScopes: append([]string(nil), input.ImplicitScopes...),
	})
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	defer writer.Rollback()
	started := time.Now()
	result, err := runner.Run(ctx, tableaucatalog.RunRequest{RequestedScopes: requested}, catalogBatchWriter{writer: writer})
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	if !slices.Equal(scopeStrings(result.RequestedScopes), input.RequestedScopes) || !slices.Equal(scopeStrings(result.ImplicitScopes), input.ImplicitScopes) {
		return catalogrefresh.HydrationResult{}, errors.New("catalog collector returned a different scope plan")
	}
	collected := append(append([]tableaucatalog.Scope(nil), result.RequestedScopes...), result.ImplicitScopes...)
	var warnings []string
	if result.DeniedPermissions > 0 {
		collected = slices.DeleteFunc(collected, func(scope tableaucatalog.Scope) bool { return scope == tableaucatalog.ScopePermissions })
		if err := writer.MarkPermissionsIncomplete(ctx); err != nil {
			return catalogrefresh.HydrationResult{}, err
		}
		warnings = append(warnings, fmt.Sprintf("Catalog permission coverage is incomplete: %d workbook permission reads were denied (HTTP 403). Missing rules are unknown, not empty permissions; readable inventory was retained.", result.DeniedPermissions))
	}
	if err := writer.CompleteScopes(ctx, scopeStrings(collected)); err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	published, err := writer.Publish(ctx)
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	counts, total, err := catalogScopeCounts(plan.Collected, result.Counts)
	if err != nil {
		return catalogrefresh.HydrationResult{}, err
	}
	if result.Requests > math.MaxInt {
		return catalogrefresh.HydrationResult{}, errors.New("catalog request count exceeds the receipt bound")
	}
	return catalogrefresh.HydrationResult{
		GenerationID: published.GenerationID, GeneratedAt: generatedAt, Complete: result.DeniedPermissions == 0, Source: catalogSourceName,
		Path: published.Path, RecordCount: published.RecordCount, HydratedRecordCount: total,
		RequestedScopes: append([]string(nil), input.RequestedScopes...), ImplicitScopes: append([]string(nil), input.ImplicitScopes...),
		ScopeCounts:       counts,
		DeniedPermissions: result.DeniedPermissions, Warnings: warnings,
		Diagnostics: catalogrefresh.Diagnostics{Requests: int(result.Requests), FailedRequests: result.DeniedPermissions, Duration: time.Since(started).Round(time.Millisecond).String()},
	}, nil
}

func catalogScopes(values []string) ([]tableaucatalog.Scope, error) {
	result := make([]tableaucatalog.Scope, len(values))
	for index, value := range values {
		scope := tableaucatalog.Scope(value)
		if _, ok := tableaucatalog.ColumnsForScope(scope); !ok {
			return nil, fmt.Errorf("catalog scope %q is unsupported", value)
		}
		result[index] = scope
	}
	return result, nil
}

func scopeStrings(values []tableaucatalog.Scope) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func catalogScopeCounts(scopes []tableaucatalog.Scope, counts map[tableaucatalog.Scope]int64) ([]catalogrefresh.ScopeCount, int, error) {
	result := make([]catalogrefresh.ScopeCount, 0, len(scopes))
	total := 0
	for _, scope := range scopes {
		count := counts[scope]
		if count < 0 || count > math.MaxInt-int64(total) {
			return nil, 0, errors.New("catalog record count exceeds the receipt bound")
		}
		total += int(count)
		result = append(result, catalogrefresh.ScopeCount{Scope: string(scope), Records: int(count)})
	}
	return result, total, nil
}

type catalogBatchWriter struct{ writer *corecatalog.GenerationWriter }

func (w catalogBatchWriter) WriteBatch(ctx context.Context, batch tableaucatalog.Batch) error {
	if w.writer == nil {
		return errors.New("catalog generation writer is not configured")
	}
	columns := make([]string, len(batch.Columns))
	for index, column := range batch.Columns {
		columns[index] = column.Name
	}
	return w.writer.WriteBatch(ctx, corecatalog.Batch{Scope: string(batch.Scope), Columns: columns, Rows: batch.Rows})
}

type catalogTableauExecutor struct {
	transport *tableau.Transport
	session   coreauth.Session
	serverURL string
	siteLUID  string
}

func (e catalogTableauExecutor) Do(ctx context.Context, input tableaucatalog.Request) (tableaucatalog.Response, error) {
	if e.transport == nil || e.session == nil || strings.TrimSpace(e.serverURL) == "" || strings.TrimSpace(e.siteLUID) == "" {
		return tableaucatalog.Response{}, errors.New("authenticated catalog transport is not configured")
	}
	response, err := e.transport.Do(ctx, e.session, tableau.Request{
		Method: http.MethodGet, ServerURL: e.serverURL,
		Path:  fmt.Sprintf("/api/%s/sites/%s%s", e.transport.APIVersion(), url.PathEscape(e.siteLUID), input.Path),
		Query: input.Query, Accept: "application/xml", Operation: input.Operation, MaxResponseBytes: input.MaxResponseBytes,
	})
	if err != nil {
		return tableaucatalog.Response{}, err
	}
	return tableaucatalog.Response{StatusCode: response.StatusCode, Body: response.Body, TableauRequestID: response.TableauRequestID}, nil
}

type catalogStoreStatuser struct{ store *corecatalog.Store }

func (s catalogStoreStatuser) Status(ctx context.Context, input catalogstatus.Input) (catalogstatus.Result, error) {
	result, err := s.store.Status(ctx, corecatalog.Selection{Environment: input.Environment, Site: input.Site, SiteSelected: input.SiteResolved})
	if err != nil {
		return catalogstatus.Result{}, err
	}
	if result.GenerationID == "" {
		return catalogstatus.Result{Environment: result.Environment, Site: result.Site, Path: result.Path}, nil
	}
	return catalogstatus.Result{ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Age: result.Age.String(), Complete: result.Complete, Stale: result.Stale, Source: result.Source, Path: result.Path, Records: result.RecordCount, Warnings: append([]string(nil), result.Warnings...)}, nil
}

type catalogGroup2Commands struct{ runtime *runtimeDependencies }

func newCatalogGroup2Commands(runtime *runtimeDependencies) *catalogGroup2Commands {
	return &catalogGroup2Commands{runtime: runtime}
}

func (c *catalogGroup2Commands) store() *corecatalog.Store {
	return corecatalog.NewStore(filepath.Dir(c.runtime.configPath), c.runtime.now)
}

func (c *catalogGroup2Commands) resolve(inputEnvironment, inputSite, operation string) (config.Environment, error) {
	_, environment, err := c.runtime.environment(inputEnvironment, false)
	if err != nil {
		return environment, capabilitySetupError(operation+".setup", operation, inputEnvironment, inputSite, "Catalog operation setup failed.", "Review the selected environment and catalog configuration.", err)
	}
	if inputSite != "" && inputSite != environment.SiteContentURL {
		return environment, capabilitySetupError(operation+".setup", operation, environment.Alias, inputSite, "Catalog source site does not match the selected environment.", "Choose the configured exact site, then retry.", errors.New("catalog source site mismatch"))
	}
	return environment, nil
}

func (c *catalogGroup2Commands) refresher() *catalogRefreshService {
	return &catalogRefreshService{commands: c}
}
func (c *catalogGroup2Commands) statuser() *catalogStatusService {
	return &catalogStatusService{commands: c}
}

type catalogRefreshService struct{ commands *catalogGroup2Commands }

func (s *catalogRefreshService) Execute(ctx context.Context, input catalogrefresh.Input) (catalogrefresh.Output, error) {
	if err := catalogrefresh.ValidateInput(input); err != nil {
		return catalogrefresh.Output{}, err
	}
	environment, err := s.commands.resolve(input.Environment, input.Site, "catalog.refresh")
	if err != nil {
		return catalogrefresh.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	hydrator := catalogHydrator{
		store: s.commands.store(), now: s.commands.runtime.now,
		executorFor: func(ctx context.Context, alias, site string) (tableaucatalog.Executor, error) {
			connection, err := s.commands.runtime.tableauConnection(ctx, alias, false)
			if err != nil {
				return nil, remoteSetupError("catalog.refresh", alias, site, connection.environment, err)
			}
			if connection.environment.SiteContentURL != site {
				return nil, errors.New("catalog refresh authenticated to a different site")
			}
			return catalogTableauExecutor{transport: connection.transport, session: connection.session, serverURL: connection.environment.URL, siteLUID: connection.session.SiteLUID()}, nil
		},
		newRunner: func(executor tableaucatalog.Executor) (catalogRunner, error) {
			return tableaucatalog.NewEngine(executor, tableaucatalog.Config{MaxConcurrency: environment.CatalogMaxConcurrency})
		},
	}
	return catalogrefresh.New(hydrator).Execute(ctx, input)
}

type catalogStatusService struct{ commands *catalogGroup2Commands }

func (s *catalogStatusService) Execute(ctx context.Context, input catalogstatus.Input) (catalogstatus.Output, error) {
	environment, err := s.commands.resolve(input.Environment, input.Site, "catalog.status")
	if err != nil {
		return catalogstatus.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	return catalogstatus.New(catalogStoreStatuser{store: s.commands.store()}).Execute(ctx, input)
}
