package inventory

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

// CacheReadSource describes the observed coverage of one cached resource query.
func CacheReadSource(result cache.ResourceResult) *readsource.Metadata {
	observed := result.NewestObserved
	if observed.IsZero() {
		observed = result.GeneratedAt
	}
	value := readsource.Cached(observed, result.Coverage, result.GenerationID, result.GeneratedAt, result.Stale)
	value.CacheWarning = result.InventoryWarning
	return &value
}

// CacheRecordSource distinguishes a confirmed detail record from a summary row.
func CacheRecordSource(result cache.ResourceResult, entry cache.ResourceEntry) *readsource.Metadata {
	result.Coverage = readsource.CoveragePartial
	if entry.Coverage == "detail" {
		result.Coverage = readsource.CoverageComplete
	}
	source := CacheReadSource(result)
	if result.Coverage == readsource.CoveragePartial {
		source.CoverageReason = "summary_only"
	}
	return source
}

// CacheReadError preserves operation-specific recovery without a live fallback.
func CacheReadError(operation, environment, site string, err error) error {
	id, summary := "cache.read_failed", "Cache read failed."
	kind := errs.KindOperation
	var uninitialized interface{ CacheUninitialized() bool }
	var unavailable interface{ CacheScopeUnavailable() bool }
	var missing interface{ CacheResourceNotFound() bool }
	var ambiguous interface{ AmbiguousCacheSelector() bool }
	var invalidCursor interface{ InvalidCacheCursor() bool }
	var refreshRequired interface{ CacheProjectRefreshRequired() bool }
	var schemaRefreshRequired interface{ CacheSchemaRefreshRequired() bool }
	var incompatible interface{ CacheSchemaIncompatible() bool }
	if errors.As(err, &refreshRequired) && refreshRequired.CacheProjectRefreshRequired() {
		return &errs.Error{ID: "cache.project_filter_unavailable", Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: "The cache lacks complete project identity coverage for this filter.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Run " + commandhint.Command("cache", "refresh", "--environment", environment, "--scope", "projects,datasources") + ", then repeat the same --cache command."}
	}
	correctiveAction := cacheReadRecovery(operation, environment)
	switch {
	case errors.As(err, &incompatible) && incompatible.CacheSchemaIncompatible():
		id, summary = "cache.schema_incompatible", "The cache schema is inconsistent or unsupported by this build."
		correctiveAction = "Preserve the cache and repair its schema or use a compatible TADX version. For a live answer, explicitly repeat the command without --cache; an identical refresh is not a repair."
	case errors.As(err, &schemaRefreshRequired) && schemaRefreshRequired.CacheSchemaRefreshRequired():
		id, summary = "cache.schema_refresh_required", "The cache schema requires an explicit refresh before cached reads can continue."
		refresh := cacheScopeRefreshCommand(operation, environment)
		if refresh == "" {
			refresh = commandhint.Environment(environment, "cache", "refresh")
		}
		correctiveAction = "Run " + refresh + " to rebuild the cache. Include any other inventory scopes you still need because rebuilding replaces the old cached data. Then repeat the --cache command."
		if operation == "datasource.schema" || strings.HasPrefix(operation, "pulse.") {
			correctiveAction = "Run " + refresh + " to rebuild the cache. Include any other inventory scopes you still need because rebuilding replaces the old cached data. Then run this command without --cache to retrieve and cache its projection before retrying the cached read."
		}
	case errors.As(err, &uninitialized) && uninitialized.CacheUninitialized():
		id, summary = "cache.uninitialized", "The cache is not initialized for this environment and site."
	case errors.As(err, &unavailable) && unavailable.CacheScopeUnavailable():
		id, summary = "cache.scope_not_indexed", "The requested resource scope is not indexed in the cache."
	case errors.As(err, &missing) && missing.CacheResourceNotFound():
		id, summary, kind = "cache.record_not_found", "No cache record matched the selector.", errs.KindUsage
	case errors.As(err, &ambiguous) && ambiguous.AmbiguousCacheSelector():
		id, summary, kind = "cache.selector_ambiguous", "The cache selector matched more than one record.", errs.KindUsage
		correctiveAction = "Use an exact LUID or a selector that identifies one resource; refreshing the cache does not resolve an ambiguous name or path."
	case errors.As(err, &invalidCursor) && invalidCursor.InvalidCacheCursor():
		id, summary, kind = "cache.cursor_invalid", "The cache continuation cursor no longer identifies the current snapshot.", errs.KindUsage
		correctiveAction = "Repeat the same --cache command without the legacy cursor to read the current snapshot."
	}
	return &errs.Error{ID: id, Kind: kind, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: err, Retryable: errs.Bool(false), CorrectiveAction: correctiveAction}
}

func cacheReadRecovery(operation, environment string) string {
	live := "Run this command without --cache for a live answer."
	if operation == "datasource.schema" {
		return live + " The live schema read attempts to cache the requested field and table metadata; inventory refresh does not collect datasource schemas."
	}
	if strings.HasPrefix(operation, "pulse.") {
		return live + " The live read attempts to cache the requested Pulse records; inventory refresh does not collect Pulse records."
	}
	if refresh := cacheScopeRefreshCommand(operation, environment); refresh != "" {
		return live + " To refresh the cached inventory, run " + refresh + ". Then repeat the same --cache command. Include other inventory scopes you still need because refresh replaces the current generation."
	}
	return live
}

func cacheScopeRefreshCommand(operation, environment string) string {
	resource, _, _ := strings.Cut(strings.TrimPrefix(operation, "admin."), ".")
	switch resource {
	case "workbook", "datasource", "flow", "project", "user", "group":
		return commandhint.Command("cache", "refresh", "--environment", environment, "--scope", resource+"s")
	default:
		return ""
	}
}

// UnsupportedCacheFilters reports filters that cannot be answered from the cache.
func UnsupportedCacheFilters(operation, environment, site string) error {
	return &errs.Error{ID: "cache.filters_unsupported", Kind: errs.KindUsage, Operation: operation, Environment: environment, Site: site, Summary: "The selected filters are not indexed for this cache read.", Retryable: errs.Bool(false), CorrectiveAction: "Run the command without --cache to use Tableau filters."}
}

// Legacy process-boundary cursors can still target a previously stored snapshot.
func LegacyInventoryCursor(value string) bool {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	for _, key := range []string{"c", "Snapshot"} {
		var token string
		if json.Unmarshal(fields[key], &token) == nil && token != "" {
			return true
		}
	}
	return false
}
