package search

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/ahillspace/tadx/internal/value"
)

type ListPager interface {
	SearchPage(context.Context, string, string, int, value.SearchRequest) (value.SearchPage, error)
}

type CompleteLists struct{ Lister ListPager }

type completeListSearchCursor struct {
	Version      int    `json:"v"`
	Fingerprint  string `json:"f"`
	TypeIndex    int    `json:"t"`
	SourceCursor string `json:"c,omitempty"`
	SourceLimit  int    `json:"l,omitempty"`
}

type invalidCompleteListSearchCursor struct{}

func (invalidCompleteListSearchCursor) Error() string {
	return "complete list search cursor is invalid"
}
func (invalidCompleteListSearchCursor) InvalidSearchCursor() bool { return true }

func (a *CompleteLists) Search(ctx context.Context, input value.SearchRequest) (value.SearchPage, error) {
	if a == nil || a.Lister == nil || len(input.Types) == 0 || input.Limit < 1 || input.Limit > 100 {
		return value.SearchPage{}, errors.New("complete live search requires configured list services, types, and a bounded limit")
	}
	types := append([]string(nil), input.Types...)
	if len(types) == 1 {
		return a.Lister.SearchPage(ctx, types[0], input.Cursor, input.Limit, input)
	}
	slices.Sort(types)
	state := completeListSearchCursor{Version: 1, Fingerprint: completeListSearchFingerprint(input)}
	if input.Cursor != "" {
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if len(input.Cursor) > 12000 || err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Fingerprint != completeListSearchFingerprint(input) || state.TypeIndex < 0 || state.TypeIndex >= len(types) || (state.SourceCursor != "" && (state.SourceLimit < 1 || state.SourceLimit > input.Limit)) || (state.SourceCursor == "" && state.SourceLimit != 0) {
			return value.SearchPage{}, invalidCompleteListSearchCursor{}
		}
	}
	result := value.SearchPage{Items: []value.SearchItem{}}
	for state.TypeIndex < len(types) && len(result.Items) < input.Limit {
		remaining := input.Limit - len(result.Items)
		sourceLimit := remaining
		if state.SourceCursor != "" {
			sourceLimit = state.SourceLimit
		}
		page, err := a.Lister.SearchPage(ctx, types[state.TypeIndex], state.SourceCursor, sourceLimit, input)
		if err != nil {
			return value.SearchPage{}, err
		}
		if len(page.Items) > remaining {
			return value.SearchPage{}, errors.New("complete live search list service exceeded the requested result bound")
		}
		result.UnresolvedMoreAvailable = result.UnresolvedMoreAvailable || page.UnresolvedMoreAvailable || (page.MoreAvailable && page.NextCursor == "")
		result.MoreAvailable = result.UnresolvedMoreAvailable
		result.Items = append(result.Items, page.Items...)
		if len(types) == 1 {
			result.Total = page.Total
		}
		result.Warnings = append(result.Warnings, page.Warnings...)
		if result.Source == "" {
			result.Source = page.Source
		} else if page.Source != "" && result.Source != page.Source {
			result.Source = "mixed"
		}
		if page.TableauRequestID != "" {
			result.TableauRequestID = page.TableauRequestID
		}
		if page.NextCursor != "" {
			state.SourceCursor = page.NextCursor
			state.SourceLimit = sourceLimit
			result.NextCursor = encodeCompleteListSearchCursor(state)
			return result, nil
		}
		state.TypeIndex++
		state.SourceCursor = ""
		state.SourceLimit = 0
	}
	if state.TypeIndex < len(types) {
		result.NextCursor = encodeCompleteListSearchCursor(state)
	}
	return result, nil
}

func completeListSearchFingerprint(input value.SearchRequest) string {
	input.Cursor = ""
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeCompleteListSearchCursor(state completeListSearchCursor) string {
	data, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(data)
}
