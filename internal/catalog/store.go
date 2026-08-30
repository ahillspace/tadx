// Package catalog defines the frozen local normalized catalog search contract.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	staleAfter   = 12 * time.Hour
)

// Record is one normalized catalog identity projection.
type Record struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// Generation is one complete, immutable catalog generation.
type Generation struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	Site        string    `json:"site"`
	GeneratedAt time.Time `json:"generated_at"`
	Complete    bool      `json:"complete"`
	Records     []Record  `json:"records"`
}

// Query contains exact filters plus an optional text search.
type Query struct {
	Text        string
	Kind        string
	ProjectPath string
	Owner       string
	Environment string
	Site        string
	LUID        string
	Cursor      string
	Limit       int
}

// Page is the bounded local continuation envelope.
type Page struct {
	Returned   int
	Total      int
	Limit      int
	NextCursor string
}

// SearchResult includes source generation and staleness provenance.
type SearchResult struct {
	Page         Page
	GenerationID string
	Environment  string
	Site         string
	GeneratedAt  time.Time
	Stale        bool
	Records      []Record
	Warnings     []string
}

// FileStore reads complete normalized generations from <root>/catalog/<environment>.json.
type FileStore struct {
	root string
	now  func() time.Time
}

// NewFileStore creates a local catalog reader.
func NewFileStore(root string, now func() time.Time) *FileStore {
	if now == nil {
		now = time.Now
	}
	return &FileStore{root: root, now: now}
}

// Search returns a deterministic bounded slice from one complete generation.
func (s *FileStore) Search(ctx context.Context, query Query) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if query.Environment == "" {
		return SearchResult{}, errors.New("catalog search requires an environment")
	}
	if strings.ContainsAny(query.Environment, `/\\`) || query.Environment == "." || query.Environment == ".." {
		return SearchResult{}, errors.New("catalog environment alias contains a path separator")
	}
	path := filepath.Join(s.root, "catalog", query.Environment+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return SearchResult{}, fmt.Errorf("read catalog generation for environment %q: %w", query.Environment, err)
	}
	var generation Generation
	if err := json.Unmarshal(data, &generation); err != nil {
		return SearchResult{}, fmt.Errorf("decode catalog generation: %w", err)
	}
	if !generation.Complete {
		return SearchResult{}, fmt.Errorf("catalog generation %q is incomplete", generation.ID)
	}
	if generation.Environment != query.Environment {
		return SearchResult{}, fmt.Errorf("catalog source environment %q does not match selected environment %q", generation.Environment, query.Environment)
	}
	if query.Site != "" && generation.Site != query.Site {
		return SearchResult{}, fmt.Errorf("catalog source site %q does not match selected site %q", generation.Site, query.Site)
	}
	items := make([]Record, 0, len(generation.Records))
	text := strings.ToLower(query.Text)
	for _, record := range generation.Records {
		if query.LUID != "" && record.LUID != query.LUID {
			continue
		}
		if query.Kind != "" && record.Kind != query.Kind {
			continue
		}
		if query.ProjectPath != "" && record.ProjectPath != query.ProjectPath {
			continue
		}
		if query.Owner != "" && record.Owner != query.Owner {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(record.Name+" "+record.ProjectPath+" "+record.Owner+" "+record.LUID), text) {
			continue
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		if items[i].ProjectPath != items[j].ProjectPath {
			return items[i].ProjectPath < items[j].ProjectPath
		}
		return items[i].LUID < items[j].LUID
	})
	limit := query.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return SearchResult{}, fmt.Errorf("catalog search limit must be between 1 and %d", maxLimit)
	}
	offset := 0
	if query.Cursor != "" {
		offset, err = strconv.Atoi(query.Cursor)
		if err != nil || offset < 0 || offset > len(items) {
			return SearchResult{}, errors.New("catalog search cursor is invalid")
		}
	}
	end := min(offset+limit, len(items))
	pageItems := append([]Record(nil), items[offset:end]...)
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	stale := s.now().Sub(generation.GeneratedAt) > staleAfter
	var warnings []string
	if stale {
		warnings = []string{fmt.Sprintf("catalog generation %q is older than 12 hours", generation.ID)}
	}
	return SearchResult{
		Page:         Page{Returned: len(pageItems), Total: len(items), Limit: limit, NextCursor: next},
		GenerationID: generation.ID, Environment: generation.Environment, Site: generation.Site, GeneratedAt: generation.GeneratedAt, Stale: stale,
		Records: pageItems, Warnings: warnings,
	}, nil
}
