package search

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/value"
)

type PageReader interface {
	Search(context.Context, value.SearchRequest) (value.SearchPage, error)
}

type BoundedPageReader interface {
	PageReader
	SearchBounded(context.Context, value.SearchRequest, int) (value.SearchPage, error)
}

type LiveSource struct {
	Native    PageReader
	Lists     PageReader
	Dedicated BoundedPageReader
}

func (s LiveSource) Search(ctx context.Context, input Input, types []string) (Result, error) {
	resourceInput := value.SearchRequest{Types: types, Terms: input.Terms, ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: input.Cursor, Limit: input.Limit}
	if strings.TrimSpace(input.Terms) == "" {
		if CompleteListSelector(input.Type) {
			return executeSource(ctx, s.Lists, resourceInput)
		}
		return executeSource(ctx, s.Dedicated, resourceInput)
	}
	if dedicatedSearchSelector(input.Type) {
		return executeSource(ctx, s.Dedicated, resourceInput)
	}
	if contentSearchSelector(input.Type) {
		return executeSource(ctx, s.Native, resourceInput)
	}
	return s.searchAll(ctx, input)
}

func executeSource(ctx context.Context, adapter PageReader, input value.SearchRequest) (Result, error) {
	if adapter == nil {
		return Result{}, errors.New("live search adapter is not configured")
	}
	page, err := adapter.Search(ctx, input)
	if err != nil {
		return Result{}, err
	}
	return SourceResult(page, nil), nil
}

func dedicatedSearchSelector(selector string) bool {
	switch selector {
	case "admin", "user", "group", "pulse", "definition", "metric":
		return true
	default:
		return false
	}
}

func contentSearchSelector(selector string) bool {
	switch selector {
	case "content", "workbook", "datasource", "flow", "project":
		return true
	default:
		return false
	}
}

const (
	combinedSearchContent   = "content"
	combinedSearchDedicated = "dedicated"
	maxCombinedCursorBytes  = 8192
	maxCombinedSourceBytes  = 4096
)

type combinedSearchCursor struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	Phase       string `json:"p"`
	Source      string `json:"c,omitempty"`
	Checksum    string `json:"s"`
}

type invalidCombinedSearchCursor struct{}

func (invalidCombinedSearchCursor) Error() string             { return "combined live search cursor is invalid" }
func (invalidCombinedSearchCursor) InvalidSearchCursor() bool { return true }

func (s LiveSource) searchAll(ctx context.Context, input Input) (Result, error) {
	if s.Native == nil || s.Dedicated == nil {
		return Result{}, errors.New("live search adapters are not configured")
	}
	state, err := decodeCombinedSearchCursor(input.Cursor, input)
	if err != nil {
		return Result{}, err
	}
	if state.Phase == combinedSearchContent {
		page, err := s.Native.Search(ctx, value.SearchRequest{
			Types: []string{"datasource", "flow", "project", "workbook"}, Terms: input.Terms,
			ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: state.Source, Limit: input.Limit,
		})
		if err != nil {
			return Result{}, err
		}
		if page.NextCursor != "" {
			if len(page.NextCursor) > maxCombinedSourceBytes {
				return Result{}, errors.New("native search continuation exceeded the combined cursor bound")
			}
			page.Total = 0
			page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchContent, Source: page.NextCursor})
			return SourceResult(page, nil), nil
		}
		if len(page.Items) == input.Limit {
			page.Total = 0
			page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchDedicated})
			return SourceResult(page, nil), nil
		}
		state.Phase = combinedSearchDedicated
		state.Source = ""
		remaining := input.Limit - len(page.Items)
		dedicatedPage, err := executeBoundedSource(ctx, s.Dedicated, value.SearchRequest{
			Types: []string{"definition", "group", "metric", "user"}, Terms: input.Terms,
			ProjectPath: input.ProjectPath, Owner: input.Owner, Limit: input.Limit,
		}, remaining)
		if err != nil {
			return Result{}, err
		}
		page.Items = append(page.Items, dedicatedPage.Items...)
		page.Total = 0
		page.Warnings = append(page.Warnings, dedicatedPage.Warnings...)
		if dedicatedPage.TableauRequestID != "" {
			page.TableauRequestID = dedicatedPage.TableauRequestID
		}
		if dedicatedPage.NextCursor != "" {
			if len(dedicatedPage.NextCursor) > maxCombinedSourceBytes {
				return Result{}, errors.New("dedicated search continuation exceeded the combined cursor bound")
			}
			page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchDedicated, Source: dedicatedPage.NextCursor})
		}
		return SourceResult(page, nil), nil
	}
	page, err := s.Dedicated.Search(ctx, value.SearchRequest{
		Types: []string{"definition", "group", "metric", "user"}, Terms: input.Terms,
		ProjectPath: input.ProjectPath, Owner: input.Owner, Cursor: state.Source, Limit: input.Limit,
	})
	if err != nil {
		return Result{}, err
	}
	page.Total = 0
	if page.NextCursor != "" {
		if len(page.NextCursor) > maxCombinedSourceBytes {
			return Result{}, errors.New("dedicated search continuation exceeded the combined cursor bound")
		}
		page.NextCursor = encodeCombinedSearchCursor(combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchDedicated, Source: page.NextCursor})
	}
	return SourceResult(page, nil), nil
}

func executeBoundedSource(ctx context.Context, adapter BoundedPageReader, input value.SearchRequest, budget int) (value.SearchPage, error) {
	if budget == input.Limit {
		return adapter.Search(ctx, input)
	}
	return adapter.SearchBounded(ctx, input, budget)
}

func decodeCombinedSearchCursor(value string, input Input) (combinedSearchCursor, error) {
	if value == "" {
		return combinedSearchCursor{Version: 1, Fingerprint: combinedSearchFingerprint(input), Phase: combinedSearchContent}, nil
	}
	if len(value) > maxCombinedCursorBytes {
		return combinedSearchCursor{}, invalidCombinedSearchCursor{}
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	var state combinedSearchCursor
	if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Fingerprint != combinedSearchFingerprint(input) || (state.Phase != combinedSearchContent && state.Phase != combinedSearchDedicated) || state.Checksum != combinedSearchChecksum(state) || (state.Phase == combinedSearchContent && state.Source == "") {
		return combinedSearchCursor{}, invalidCombinedSearchCursor{}
	}
	return state, nil
}

func encodeCombinedSearchCursor(state combinedSearchCursor) string {
	state.Checksum = combinedSearchChecksum(state)
	data, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(data)
}

func combinedSearchFingerprint(input Input) string {
	input.Cursor = ""
	data, _ := json.Marshal(input)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func combinedSearchChecksum(state combinedSearchCursor) string {
	state.Checksum = ""
	data, _ := json.Marshal(state)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func SourceResult(page value.SearchPage, generation *Generation) Result {
	items := make([]Item, len(page.Items))
	for index, item := range page.Items {
		items[index] = Item{LUID: item.LUID, Type: item.Type, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner, ModifiedAt: item.ModifiedAt}
	}
	return Result{Items: items, Page: Page{NextCursor: page.NextCursor, Total: page.Total, MoreAvailable: page.MoreAvailable, UnresolvedMoreAvailable: page.UnresolvedMoreAvailable}, Warnings: page.Warnings, Generation: generation, Source: page.Source}
}
