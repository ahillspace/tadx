// Package search merges bounded resource inventory pages without a search API.
package search

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const pageSize = 100
const maxPages = 100

type Item struct{ LUID, Type, Name, ProjectPath, Owner, ModifiedAt string }
type Page struct {
	UnresolvedMoreAvailable bool
	Total                   int
	Items                   []Item
	NextCursor              string
	Warnings                []string
	TableauRequestID        string
	Source                  string
	MoreAvailable           bool
}
type Input struct {
	Types                             []string
	Terms, ProjectPath, Owner, Cursor string
	Limit                             int
}

// Source supplies one bounded resource page with stable upstream continuation.
type Source interface {
	List(context.Context, string, string, int) (Page, error)
}
type Adapter struct{ source Source }

func NewAdapter(source Source) *Adapter { return &Adapter{source: source} }

type cursor struct {
	Version      int    `json:"v"`
	Fingerprint  string `json:"f"`
	TypeIndex    int    `json:"t"`
	SourceCursor string `json:"c,omitempty"`
	Offset       int    `json:"o,omitempty"`
	PageDigest   string `json:"p,omitempty"`
}
type cursorError struct{}

func (cursorError) Error() string             { return "search cursor is invalid or its source page changed" }
func (cursorError) InvalidSearchCursor() bool { return true }

// Search streams selected types and preserves progress when the scan budget ends.
// Type order is lexical; items within each upstream page sort by name and LUID.
func (a *Adapter) Search(ctx context.Context, input Input) (Page, error) {
	return a.SearchBounded(ctx, input, input.Limit)
}

// SearchBounded returns at most budget rows while binding continuation to the
// original input limit. This lets a composed source fill a partial output page,
// then resume with the caller's normal limit without weakening cursor checks.
func (a *Adapter) SearchBounded(ctx context.Context, input Input, budget int) (Page, error) {
	if a == nil || a.source == nil {
		return Page{}, errors.New("search adapter is not configured")
	}
	if input.Limit < 1 || input.Limit > 100 || budget < 1 || budget > input.Limit || len(input.Types) == 0 || len(input.Types) > 8 {
		return Page{}, errors.New("search requires bounded types and limit")
	}
	input.Types = append([]string{}, input.Types...)
	sort.Strings(input.Types)
	for i, kind := range input.Types {
		if !validType(kind) || (i > 0 && input.Types[i-1] == kind) {
			return Page{}, errors.New("invalid or duplicate search type")
		}
	}
	state := cursor{Version: 1, Fingerprint: fingerprint(input)}
	if input.Cursor != "" {
		if len(input.Cursor) > 12000 {
			return Page{}, cursorError{}
		}
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Fingerprint != fingerprint(input) || state.TypeIndex < 0 || state.TypeIndex >= len(input.Types) || state.Offset < 0 || (state.Offset > 0 && state.PageDigest == "") {
			return Page{}, cursorError{}
		}
	}
	result := Page{Items: []Item{}}
	seenCursors := make(map[string]bool)
	for requests := 0; state.TypeIndex < len(input.Types) && requests < maxPages; requests++ {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		kind := input.Types[state.TypeIndex]
		key := kind + ":" + state.SourceCursor
		if seenCursors[key] {
			return Page{}, errors.New("search source repeated a continuation cursor")
		}
		seenCursors[key] = true
		page, err := a.source.List(ctx, kind, state.SourceCursor, pageSize)
		if err != nil {
			return Page{}, err
		}
		if len(page.Items) > pageSize {
			return Page{}, errors.New("search source exceeded the page bound")
		}
		if page.NextCursor != "" && page.NextCursor == state.SourceCursor {
			return Page{}, errors.New("search source repeated a continuation cursor")
		}
		page.Items = append([]Item{}, page.Items...)
		seen := make(map[string]bool, len(page.Items))
		for _, item := range page.Items {
			if item.Type != kind || strings.TrimSpace(item.LUID) == "" || (strings.TrimSpace(item.Name) == "" && kind != "metric") || seen[item.LUID] {
				return Page{}, errors.New("search source returned invalid authoritative identity")
			}
			seen[item.LUID] = true
		}
		sort.Slice(page.Items, func(i, j int) bool {
			if page.Items[i].Name != page.Items[j].Name {
				return page.Items[i].Name < page.Items[j].Name
			}
			return page.Items[i].LUID < page.Items[j].LUID
		})
		digest := pageDigest(page)
		if state.Offset > len(page.Items) || (state.PageDigest != "" && state.PageDigest != digest) {
			return Page{}, cursorError{}
		}
		result.Warnings = append(result.Warnings, page.Warnings...)
		if len(input.Types) == 1 && strings.TrimSpace(input.Terms) == "" && strings.TrimSpace(input.ProjectPath) == "" && strings.TrimSpace(input.Owner) == "" {
			result.Total = page.Total
		}
		for i := state.Offset; i < len(page.Items); i++ {
			item := page.Items[i]
			if matches(item, input) {
				result.Items = append(result.Items, item)
			}
			if len(result.Items) == budget {
				if i+1 < len(page.Items) {
					state.Offset = i + 1
					state.PageDigest = digest
				} else {
					advance(&state, page.NextCursor)
				}
				if state.TypeIndex < len(input.Types) {
					result.NextCursor = encode(state)
				}
				return result, nil
			}
		}
		advance(&state, page.NextCursor)
	}
	if state.TypeIndex < len(input.Types) {
		result.NextCursor = encode(state)
		result.Warnings = append(result.Warnings, "Search reached its bounded scan limit; narrow the search or increase --limit to inspect more matching resources.")
	}
	return result, nil
}

func advance(state *cursor, next string) {
	state.Offset = 0
	state.PageDigest = ""
	state.SourceCursor = next
	if next == "" {
		state.TypeIndex++
	}
}
func matches(item Item, input Input) bool {
	return (input.ProjectPath == "" || item.ProjectPath == input.ProjectPath) && (input.Owner == "" || item.Owner == input.Owner) && (input.Terms == "" || strings.Contains(strings.ToLower(item.Name), strings.ToLower(input.Terms)))
}
func fingerprint(input Input) string {
	input.Cursor = ""
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func pageDigest(page Page) string {
	data, _ := json.Marshal(struct {
		Items []Item
		Next  string
	}{page.Items, page.NextCursor})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func encode(state cursor) string {
	data, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(data)
}
func validType(kind string) bool {
	switch kind {
	case "workbook", "datasource", "flow", "project", "user", "group", "definition", "metric":
		return true
	}
	return false
}
