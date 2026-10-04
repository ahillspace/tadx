package search

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cache"
)

type CacheSource struct {
	Store *cache.Store
}

func (s CacheSource) Search(ctx context.Context, input searchaction.Input, types []string) (searchaction.Result, error) {
	lister := &cacheSearchLister{store: s.Store, environment: input.Environment, site: input.Site, skipUnavailable: input.Type == "", observations: make(map[string]cacheSearchObservation)}
	adapter := NewAdapter(lister)
	page, err := adapter.Search(ctx, Input{Types: types, Terms: input.Terms, ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		if unavailable, ok := errors.AsType[cacheSearchScopeUnavailable](err); ok {
			recovery, observeErr := s.observeCacheRecovery(ctx, input, types)
			if observeErr == nil {
				unavailable.recovery = recovery
				return searchaction.Result{}, unavailable
			}
		}
		return searchaction.Result{}, err
	}
	var generation *searchaction.Generation
	shared := len(lister.observations) > 0
	for _, observation := range lister.observations {
		after, err := s.Store.ReadResources(ctx, observation.query)
		if err != nil || cacheResourceFingerprint(after) != cacheResourceFingerprint(observation.result) {
			return searchaction.Result{}, invalidCacheResourceCursor{}
		}
		result := observation.result
		if result.Coverage != "complete" || result.GenerationID == "" {
			shared = false
			continue
		}
		if generation == nil {
			generation = &searchaction.Generation{ID: result.GenerationID, Environment: input.Environment, Site: input.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Stale: result.Stale}
		} else if generation.ID != result.GenerationID {
			shared = false
		}
	}
	if shared {
		return searchaction.SourceResult(page, generation), nil
	}
	page.Warnings = append(page.Warnings, "Search used partial cache records or independently refreshed resource snapshots; no shared complete generation describes this page.")
	return searchaction.SourceResult(page, nil), nil
}

func (s CacheSource) observeCacheRecovery(ctx context.Context, input searchaction.Input, required []string) (searchaction.CacheRecovery, error) {
	recovery := searchaction.CacheRecovery{Required: append([]string(nil), required...)}
	for _, resourceType := range required {
		result, err := s.Store.ReadResources(ctx, cache.ResourceQuery{Environment: input.Environment, Site: input.Site, Kind: resourceType, Limit: 1})
		if err == nil {
			recovery.Available = append(recovery.Available, searchaction.CacheTypeObservation{Type: resourceType, Coverage: result.Coverage, Stale: result.Stale, Count: result.Total})
			continue
		}
		unavailable, missingScope := errors.AsType[interface {
			error
			CacheScopeUnavailable() bool
		}](err)
		uninitialized, missingStore := errors.AsType[interface {
			error
			CacheUninitialized() bool
		}](err)
		if missingScope && unavailable.CacheScopeUnavailable() || missingStore && uninitialized.CacheUninitialized() {
			recovery.Missing = append(recovery.Missing, resourceType)
			continue
		}
		return searchaction.CacheRecovery{}, err
	}
	return recovery, nil
}

type cacheSearchObservation struct {
	query  cache.ResourceQuery
	result cache.ResourceResult
}

type cacheSearchLister struct {
	store             *cache.Store
	environment, site string
	skipUnavailable   bool
	observations      map[string]cacheSearchObservation
}

func (s *cacheSearchLister) List(ctx context.Context, resourceType, cursor string, limit int) (Page, error) {
	state, err := decodeCacheResourceCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	query := cache.ResourceQuery{Environment: s.environment, Site: s.site, Kind: resourceType, Offset: state.Offset, Limit: limit}
	result, err := s.store.ReadResources(ctx, query)
	if err != nil {
		unavailable, missingScope := errors.AsType[interface {
			error
			CacheScopeUnavailable() bool
		}](err)
		uninitialized, missingStore := errors.AsType[interface {
			error
			CacheUninitialized() bool
		}](err)
		missing := (missingScope && unavailable.CacheScopeUnavailable()) || (missingStore && uninitialized.CacheUninitialized())
		if missing {
			if s.skipUnavailable {
				return Page{Warnings: []string{"Cache does not contain " + resourceType + " resources; that type was omitted."}}, nil
			}
			return Page{}, cacheSearchScopeUnavailable{resourceType: resourceType, cause: err}
		}
		return Page{}, err
	}
	fingerprint := cacheResourceFingerprint(result)
	if s.observations != nil {
		if previous, ok := s.observations[resourceType]; ok && cacheResourceFingerprint(previous.result) != fingerprint {
			return Page{}, invalidCacheResourceCursor{}
		}
		s.observations[resourceType] = cacheSearchObservation{query: query, result: result}
	}
	if state.Fingerprint != "" && state.Fingerprint != fingerprint {
		return Page{}, invalidCacheResourceCursor{}
	}
	items := make([]Item, len(result.Entries))
	for index, item := range result.Entries {
		items[index] = Item{LUID: item.LUID, Type: item.Kind, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner}
	}
	next := ""
	if state.Offset+len(result.Entries) < result.Total {
		next = encodeCacheResourceCursor(cacheResourceCursor{Offset: state.Offset + len(result.Entries), Fingerprint: fingerprint})
	}
	return Page{Items: items, NextCursor: next, Total: result.Total}, nil
}

type cacheSearchScopeUnavailable struct {
	resourceType string
	cause        error
	recovery     searchaction.CacheRecovery
}

func (e cacheSearchScopeUnavailable) Error() string {
	return fmt.Sprintf("cache does not contain %s resources", e.resourceType)
}
func (e cacheSearchScopeUnavailable) Unwrap() error             { return e.cause }
func (cacheSearchScopeUnavailable) CacheScopeUnavailable() bool { return true }
func (e cacheSearchScopeUnavailable) CacheSearchRecovery() searchaction.CacheRecovery {
	return e.recovery
}

type cacheResourceCursor struct {
	Offset      int    `json:"o"`
	Fingerprint string `json:"f"`
}

type invalidCacheResourceCursor struct{}

func (invalidCacheResourceCursor) Error() string {
	return "cache resource cursor is invalid or stale"
}
func (invalidCacheResourceCursor) InvalidSearchCursor() bool { return true }

func decodeCacheResourceCursor(value string) (cacheResourceCursor, error) {
	if value == "" {
		return cacheResourceCursor{}, nil
	}
	if len(value) > 4096 {
		return cacheResourceCursor{}, invalidCacheResourceCursor{}
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var cursor cacheResourceCursor
	if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Offset < 1 || cursor.Fingerprint == "" {
		return cacheResourceCursor{}, invalidCacheResourceCursor{}
	}
	return cursor, nil
}

func encodeCacheResourceCursor(cursor cacheResourceCursor) string {
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func cacheResourceFingerprint(result cache.ResourceResult) string {
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
