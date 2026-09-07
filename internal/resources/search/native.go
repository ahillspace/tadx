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

	tableausearch "github.com/ahillspace/tadx/internal/tableau/search"
)

const maxNativeCursorBytes = 4096

// NativeClient is the narrow released-REST native search boundary.
type NativeClient interface {
	Search(context.Context, tableausearch.Request) (tableausearch.Page, error)
}

// DatasourceIdentityResolver translates native search datasource identities
// into classic REST LUIDs used by TADX lifecycle actions.
type DatasourceIdentityResolver interface {
	ResolveContentURLs(context.Context, []string) (map[string]string, error)
}

// NativeAdapter maps Tableau native search pages to the shared resource result.
type NativeAdapter struct {
	client             NativeClient
	datasourceResolver DatasourceIdentityResolver
}

// NewNativeAdapter creates the native content-only search adapter.
func NewNativeAdapter(client NativeClient, datasourceResolver DatasourceIdentityResolver) *NativeAdapter {
	return &NativeAdapter{client: client, datasourceResolver: datasourceResolver}
}

type nativeCursor struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	Page        int    `json:"p"`
}

// Search executes one bounded native page while keeping its numeric pagination private.
func (a *NativeAdapter) Search(ctx context.Context, input Input) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("native search adapter is not configured")
	}
	if strings.TrimSpace(input.ProjectPath) != "" || strings.TrimSpace(input.Owner) != "" {
		return Page{}, errors.New("native content search does not support TADX project-path or owner-name filters")
	}
	state := nativeCursor{Version: 1, Fingerprint: nativeFingerprint(input)}
	if input.Cursor != "" {
		if len(input.Cursor) > maxNativeCursorBytes {
			return Page{}, cursorError{}
		}
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.Fingerprint != nativeFingerprint(input) || state.Page < 1 || state.Page > 1999 {
			return Page{}, cursorError{}
		}
	}
	page, err := a.client.Search(ctx, tableauSearchRequest(input, state.Page))
	if err != nil {
		return Page{}, err
	}
	if page.PageIndex != state.Page || page.Limit != input.Limit || len(page.Items) > input.Limit {
		return Page{}, errors.New("native search response changed the requested page")
	}
	requested := make(map[string]bool, len(input.Types))
	for _, kind := range input.Types {
		requested[strings.ToLower(strings.TrimSpace(kind))] = true
	}
	contentURLs := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		if item.Type == "datasource" {
			if strings.TrimSpace(item.ContentURL) == "" || a.datasourceResolver == nil {
				return Page{}, errors.New("native datasource search result cannot be mapped to a classic REST LUID")
			}
			contentURLs = append(contentURLs, item.ContentURL)
		}
	}
	resolvedDatasources := map[string]string{}
	if len(contentURLs) > 0 {
		resolvedDatasources, err = a.datasourceResolver.ResolveContentURLs(ctx, contentURLs)
		if err != nil {
			return Page{}, err
		}
	}
	result := Page{Items: make([]Item, len(page.Items)), TableauRequestID: page.TableauRequestID, Total: page.Total}
	if page.Total > 2000 {
		result.Warnings = append(result.Warnings, "Tableau native search exposes only the first 2,000 matching results; narrow the search to inspect remaining matches.")
	}
	seen := make(map[string]struct{}, len(page.Items))
	for i, item := range page.Items {
		if item.Type == "datasource" {
			item.LUID = strings.TrimSpace(resolvedDatasources[item.ContentURL])
		}
		if strings.TrimSpace(item.LUID) == "" || strings.TrimSpace(item.Name) == "" || !requested[item.Type] {
			return Page{}, errors.New("native search returned invalid authoritative content identity")
		}
		key := item.Type + "\x00" + item.LUID
		if _, duplicate := seen[key]; duplicate {
			return Page{}, errors.New("native search returned duplicate authoritative content identity")
		}
		seen[key] = struct{}{}
		owner := item.OwnerName
		if owner == "" {
			owner = item.OwnerLUID
		}
		result.Items[i] = Item{LUID: item.LUID, Type: item.Type, Name: item.Name, ProjectPath: item.ProjectPath, Owner: owner, ModifiedAt: item.ModifiedAt}
	}
	if page.HasNext {
		state.Page++
		result.NextCursor = encodeNativeCursor(state)
	}
	return result, nil
}

func tableauSearchRequest(input Input, page int) tableausearch.Request {
	return tableausearch.Request{Terms: input.Terms, Types: append([]string{}, input.Types...), Limit: input.Limit, Page: page}
}

func nativeFingerprint(input Input) string {
	types := make([]string, 0, len(input.Types))
	seen := make(map[string]bool, len(input.Types))
	for _, candidate := range input.Types {
		kind := strings.ToLower(strings.TrimSpace(candidate))
		if !seen[kind] {
			seen[kind] = true
			types = append(types, kind)
		}
	}
	sort.Strings(types)
	data, _ := json.Marshal(struct {
		Types []string `json:"types"`
		Terms string   `json:"terms"`
		Limit int      `json:"limit"`
	}{types, strings.TrimSpace(input.Terms), input.Limit})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func encodeNativeCursor(state nativeCursor) string {
	data, _ := json.Marshal(state)
	return base64.RawURLEncoding.EncodeToString(data)
}
