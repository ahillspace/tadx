package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	cacherefresh "github.com/ahillspace/tadx/actions/cache/refresh"
	cachestatus "github.com/ahillspace/tadx/actions/cache/status"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	corecache "github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	"github.com/ahillspace/tadx/internal/value"
)

const cacheSourceName = "tableau-rest"

type cacheRunner interface {
	Run(context.Context, tableaucache.RunRequest, tableaucache.BatchWriter) (tableaucache.Result, error)
}

type cacheExecutorFactory func(context.Context, string, string) (tableaucache.Executor, error)
type cacheRunnerFactory func(tableaucache.Executor) (cacheRunner, error)

type cacheHydrator struct {
	checkCapability func(string) error
	store           *corecache.Store
	now             func() time.Time
	executorFor     cacheExecutorFactory
	newRunner       cacheRunnerFactory
}

func (h cacheHydrator) Hydrate(ctx context.Context, input cacherefresh.HydrationRequest) (cacherefresh.HydrationResult, error) {
	if h.store == nil || h.executorFor == nil || h.newRunner == nil {
		return cacherefresh.HydrationResult{}, errors.New("cache hydrator is not configured")
	}
	requested, err := cacheScopes(input.RequestedScopes)
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	plan, err := tableaucache.PlanScopes(requested)
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	if err := checkCacheScopeCapabilities(h.checkCapability, plan.Collected); err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	if !slices.Equal(scopeStrings(plan.Requested), input.RequestedScopes) || !slices.Equal(scopeStrings(plan.Implicit), input.ImplicitScopes) {
		return cacherefresh.HydrationResult{}, errors.New("cache action and collector scope plans differ")
	}
	executor, err := h.executorFor(ctx, input.Environment, input.Site)
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	runner, err := h.newRunner(executor)
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	now := h.now
	if now == nil {
		now = time.Now
	}
	generatedAt := now().UTC()
	writer, err := h.store.BeginRefreshGeneration(ctx, corecache.GenerationMetadata{
		Environment: input.Environment, Site: input.Site, GeneratedAt: generatedAt, Source: cacheSourceName,
		RequestedScopes: append([]string(nil), input.RequestedScopes...), ImplicitScopes: append([]string(nil), input.ImplicitScopes...),
	})
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	defer writer.Rollback()
	started := time.Now()
	result, err := runner.Run(ctx, tableaucache.RunRequest{RequestedScopes: requested}, cacheBatchWriter{writer: writer})
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	if !slices.Equal(scopeStrings(result.RequestedScopes), input.RequestedScopes) || !slices.Equal(scopeStrings(result.ImplicitScopes), input.ImplicitScopes) {
		return cacherefresh.HydrationResult{}, errors.New("cache collector returned a different scope plan")
	}
	collected := append(append([]tableaucache.Scope(nil), result.RequestedScopes...), result.ImplicitScopes...)
	var warnings []string
	if result.DeniedPermissions > 0 {
		collected = slices.DeleteFunc(collected, func(scope tableaucache.Scope) bool { return scope == tableaucache.ScopePermissions })
		if err := writer.MarkPermissionsIncomplete(ctx); err != nil {
			return cacherefresh.HydrationResult{}, err
		}
		warnings = append(warnings, fmt.Sprintf("Cache permission coverage is incomplete: %d workbook permission reads were denied (HTTP 403). Missing rules are unknown, not empty permissions; readable inventory was retained.", result.DeniedPermissions))
	}
	if err := writer.CompleteScopes(ctx, scopeStrings(collected)); err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	counts, total, err := cacheScopeCounts(plan.Collected, result.Counts)
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	if result.Requests > math.MaxInt {
		return cacherefresh.HydrationResult{}, errors.New("cache request count exceeds the receipt bound")
	}
	published, err := writer.Publish(ctx)
	if err != nil {
		return cacherefresh.HydrationResult{}, err
	}
	return cacherefresh.HydrationResult{
		GenerationID: published.GenerationID, GeneratedAt: generatedAt, Complete: result.DeniedPermissions == 0, Source: cacheSourceName,
		Path: published.Path, RecordCount: published.RecordCount, HydratedRecordCount: total,
		RequestedScopes: append([]string(nil), input.RequestedScopes...), ImplicitScopes: append([]string(nil), input.ImplicitScopes...),
		ScopeCounts:       counts,
		DeniedPermissions: result.DeniedPermissions, Warnings: warnings,
		Diagnostics: cacherefresh.Diagnostics{Requests: int(result.Requests), FailedRequests: result.DeniedPermissions, Duration: time.Since(started).Round(time.Millisecond).String()},
	}, nil
}

func cacheScopes(values []string) ([]tableaucache.Scope, error) {
	result := make([]tableaucache.Scope, len(values))
	for index, value := range values {
		scope := tableaucache.Scope(value)
		if _, ok := tableaucache.ColumnsForScope(scope); !ok {
			return nil, fmt.Errorf("cache scope %q is unsupported", value)
		}
		result[index] = scope
	}
	return result, nil
}

func scopeStrings(values []tableaucache.Scope) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func cacheScopeCounts(scopes []tableaucache.Scope, counts map[tableaucache.Scope]int64) ([]cacherefresh.ScopeCount, int, error) {
	result := make([]cacherefresh.ScopeCount, 0, len(scopes))
	total := 0
	for _, scope := range scopes {
		count := counts[scope]
		if count < 0 || count > math.MaxInt-int64(total) {
			return nil, 0, errors.New("cache record count exceeds the receipt bound")
		}
		total += int(count)
		result = append(result, cacherefresh.ScopeCount{Scope: string(scope), Records: int(count)})
	}
	return result, total, nil
}

type cacheBatchWriter struct{ writer *corecache.GenerationWriter }

func (w cacheBatchWriter) WriteBatch(ctx context.Context, batch tableaucache.Batch) error {
	if w.writer == nil {
		return errors.New("cache generation writer is not configured")
	}
	columns := make([]string, len(batch.Columns))
	for index, column := range batch.Columns {
		columns[index] = column.Name
	}
	return w.writer.WriteBatch(ctx, corecache.Batch{Scope: string(batch.Scope), Columns: columns, Rows: batch.Rows})
}

type cacheTableauExecutor struct {
	checkCapability func(string) error
	transport       *tableau.Transport
	session         coreauth.Session
	serverURL       string
	siteLUID        string
}

func (e cacheTableauExecutor) Do(ctx context.Context, input tableaucache.Request) (tableaucache.Response, error) {
	if err := checkCacheScopeCapabilities(e.checkCapability, []tableaucache.Scope{input.Scope}); err != nil {
		return tableaucache.Response{}, err
	}
	if e.transport == nil || e.session == nil || strings.TrimSpace(e.serverURL) == "" || strings.TrimSpace(e.siteLUID) == "" {
		return tableaucache.Response{}, errors.New("authenticated cache transport is not configured")
	}
	response, err := e.transport.Do(ctx, e.session, tableau.Request{
		Method: http.MethodGet, ServerURL: e.serverURL,
		Path:  fmt.Sprintf("/api/%s/sites/%s%s", e.transport.APIVersion(), url.PathEscape(e.siteLUID), input.Path),
		Query: input.Query, Accept: "application/xml", Operation: input.Operation, MaxResponseBytes: input.MaxResponseBytes,
	})
	if err != nil {
		return tableaucache.Response{}, err
	}
	return tableaucache.Response{StatusCode: response.StatusCode, Body: response.Body, TableauRequestID: response.TableauRequestID}, nil
}

type cacheStoreStatuser struct{ store *corecache.Store }

func (s cacheStoreStatuser) Status(ctx context.Context, input cachestatus.Input) (cachestatus.Result, error) {
	result, err := s.store.Status(ctx, corecache.Selection{Environment: input.Environment, Site: input.Site, SiteSelected: input.SiteResolved})
	if err != nil {
		return cachestatus.Result{}, err
	}
	retained := make([]value.CachedObservation, len(result.Retained))
	for i, item := range result.Retained {
		retained[i] = value.CachedObservation{Kind: item.Kind, Records: item.Records, Complete: item.Complete, Stale: item.Stale, Oldest: item.Oldest.UTC().Format(time.RFC3339Nano), Newest: item.Newest.UTC().Format(time.RFC3339Nano)}
	}
	if result.GenerationID == "" {
		return cachestatus.Result{Retained: retained, Environment: result.Environment, Site: result.Site, Path: result.Path}, nil
	}
	coverage := make([]value.CacheCoverage, len(result.Coverage))
	for i, scope := range result.Coverage {
		coverage[i] = value.CacheCoverage{Scope: scope.Scope, Requested: scope.Requested, Complete: scope.Complete, Records: scope.Records}
	}
	return cachestatus.Result{Retained: retained, Coverage: coverage, ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Age: result.Age.String(), Complete: result.Complete, Stale: result.Stale, Source: result.Source, Path: result.Path, Records: result.RecordCount, Warnings: append([]string(nil), result.Warnings...)}, nil
}

type cacheGroup2Commands struct{ runtime *runtimeDependencies }

func newCacheGroup2Commands(runtime *runtimeDependencies) *cacheGroup2Commands {
	return &cacheGroup2Commands{runtime: runtime}
}

func (c *cacheGroup2Commands) store(environment config.Environment) *corecache.Store {
	return c.runtime.cacheStore(environment)
}

func (c *cacheGroup2Commands) resolve(inputEnvironment, inputSite, operation string) (config.Environment, error) {
	_, environment, err := c.runtime.environment(inputEnvironment, false)
	if err != nil {
		return environment, capabilitySetupError(operation+".setup", operation, inputEnvironment, inputSite, "Cache operation setup failed.", "Review the selected environment and cache configuration.", err)
	}
	if inputSite != "" && inputSite != environment.SiteContentURL {
		return environment, capabilitySetupError(operation+".setup", operation, environment.Alias, inputSite, "Cache source site does not match the selected environment.", "Choose the configured exact site, then retry.", errors.New("cache source site mismatch"))
	}
	return environment, nil
}

func (c *cacheGroup2Commands) refresher() *cacheRefreshService {
	return &cacheRefreshService{commands: c}
}
func (c *cacheGroup2Commands) statuser() *cacheStatusService {
	return &cacheStatusService{commands: c}
}

type cacheRefreshService struct{ commands *cacheGroup2Commands }

func (s *cacheRefreshService) Execute(ctx context.Context, input cacherefresh.Input) (cacherefresh.Output, error) {
	if err := cacherefresh.ValidateInput(input); err != nil {
		return cacherefresh.Output{}, err
	}
	if !input.Preview {
		requested, err := cacheScopes(input.Scopes)
		if err != nil {
			return cacherefresh.Output{}, err
		}
		plan, err := tableaucache.PlanScopes(requested)
		if err != nil {
			return cacherefresh.Output{}, err
		}
		if err := checkCacheScopeCapabilities(s.commands.runtime.checkManagedCapability, plan.Collected); err != nil {
			return cacherefresh.Output{}, err
		}
	}
	environment, err := s.commands.resolve(input.Environment, input.Site, "cache.refresh")
	if err != nil {
		return cacherefresh.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	if input.Preview {
		return cacherefresh.New(nil).Execute(ctx, input)
	}
	hydrator := cacheHydrator{
		checkCapability: s.commands.runtime.checkManagedCapability,
		store:           s.commands.store(environment), now: s.commands.runtime.now,
		executorFor: func(ctx context.Context, alias, site string) (tableaucache.Executor, error) {
			connection, err := s.commands.runtime.tableauConnection(ctx, alias, false)
			if err != nil {
				return nil, remoteSetupError("cache.refresh", alias, site, connection.environment, err)
			}
			if connection.environment.SiteContentURL != site {
				return nil, errors.New("cache refresh authenticated to a different site")
			}
			return cacheTableauExecutor{checkCapability: s.commands.runtime.checkManagedCapability, transport: connection.transport, session: connection.session, serverURL: connection.environment.URL, siteLUID: connection.session.SiteLUID()}, nil
		},
		newRunner: func(executor tableaucache.Executor) (cacheRunner, error) {
			return tableaucache.NewEngine(executor, tableaucache.Config{MaxConcurrency: environment.CacheMaxConcurrency})
		},
	}
	return cacherefresh.New(hydrator).Execute(ctx, input)
}

type cacheStatusService struct{ commands *cacheGroup2Commands }

func checkCacheScopeCapabilities(check func(string) error, scopes []tableaucache.Scope) error {
	if check == nil {
		return nil
	}
	for _, scope := range scopes {
		var id string
		switch scope {
		case tableaucache.ScopeUsers:
			id = "admin.user.list"
		case tableaucache.ScopeGroups:
			id = "admin.group.list"
		case tableaucache.ScopePermissions:
			id = "admin.permission.inspect"
		}
		if id != "" {
			if err := check(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *cacheStatusService) Execute(ctx context.Context, input cachestatus.Input) (cachestatus.Output, error) {
	environment, err := s.commands.resolve(input.Environment, input.Site, "cache.status")
	if err != nil {
		return cachestatus.Output{}, err
	}
	input.Environment, input.Site, input.SiteResolved = environment.Alias, environment.SiteContentURL, true
	return cachestatus.New(cacheStoreStatuser{store: s.commands.store(environment)}).Execute(ctx, input)
}
