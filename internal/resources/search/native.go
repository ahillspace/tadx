package search

import (
	"cmp"
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
const maxNativeHits = 2000

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
	fingerprint        string
	pages              map[int]tableausearch.Page
	datasourceIDs      map[string]string
}

// NewNativeAdapter creates the native content-only search adapter.
func NewNativeAdapter(client NativeClient, datasourceResolver DatasourceIdentityResolver) *NativeAdapter {
	return &NativeAdapter{client: client, datasourceResolver: datasourceResolver}
}

type nativeCursor struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	Page        int    `json:"p"`
	Offset      int    `json:"o,omitzero"`
	Prefix      string `json:"s,omitempty"`
}

// Search projects native ranked hits into content, retaining the first occurrence
// of each published datasource. Cursors record raw progress, not content counts.
func (a *NativeAdapter) Search(ctx context.Context, input Input) (Page, error) {
	if a == nil || a.client == nil {
		return Page{}, errors.New("native search adapter is not configured")
	}
	if strings.TrimSpace(input.ProjectPath) != "" || strings.TrimSpace(input.Owner) != "" {
		return Page{}, errors.New("native content search does not support TADX project-path or owner-name filters")
	}
	if input.Limit < 1 || input.Limit > 100 {
		return Page{}, errors.New("native content search requires a limit between 1 and 100")
	}
	state := nativeCursor{Version: 2, Fingerprint: nativeFingerprint(input)}
	if input.Cursor != "" {
		if len(input.Cursor) > maxNativeCursorBytes {
			return Page{}, cursorError{}
		}
		data, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(data, &state) != nil || state.Fingerprint != nativeFingerprint(input) || state.Page < 0 || state.Page > (maxNativeHits-1)/input.Limit {
			return Page{}, cursorError{}
		}
		switch state.Version {
		case 1:
			if state.Page < 1 || state.Offset != 0 || state.Prefix != "" {
				return Page{}, cursorError{}
			}
		case 2:
			if state.Offset < 0 || state.Offset >= input.Limit || state.Page*input.Limit+state.Offset >= maxNativeHits || len(state.Prefix) != sha256.Size*2 {
				return Page{}, cursorError{}
			}
		default:
			return Page{}, cursorError{}
		}
	}
	if a.fingerprint != state.Fingerprint {
		a.fingerprint = state.Fingerprint
		a.pages = make(map[int]tableausearch.Page)
		a.datasourceIDs = make(map[string]string)
	}
	requested := make(map[string]bool, len(input.Types))
	for _, kind := range input.Types {
		requested[strings.ToLower(strings.TrimSpace(kind))] = true
	}
	result := Page{Items: []Item{}}
	seen := make(map[string]Item)
	seenHits := make(map[string]bool)
	prefix := sha256.New()
	resumeAt := state.Page*input.Limit + state.Offset
	resumed := input.Cursor == ""
	total := -1
	// Replay the prefix to reconstruct exact identities without putting an
	// unbounded identity list in the cursor. Pages and REST mappings are cached
	// only on this command's adapter, so internal continuation does not refetch.
	for pageIndex := range (maxNativeHits + input.Limit - 1) / input.Limit {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		page, err := a.nativePage(ctx, input, pageIndex)
		if err != nil {
			return Page{}, err
		}
		if total < 0 {
			total = page.Total
			if total > maxNativeHits {
				result.MoreAvailable = true
				result.Warnings = append(result.Warnings, "Tableau native search exposes only the first 2,000 matching hits; narrow the search to inspect remaining matches.")
			}
		} else if total != page.Total {
			return Page{}, errors.New("native search total changed during pagination; start a new search")
		}
		if pageIndex >= state.Page && result.TableauRequestID == "" {
			result.TableauRequestID = page.TableauRequestID
		}
		for offset, hit := range page.Items {
			position := pageIndex*input.Limit + offset
			if position >= maxNativeHits {
				break
			}
			if !resumed && position > resumeAt {
				return Page{}, cursorError{}
			}
			if position == resumeAt {
				if input.Cursor != "" && state.Version == 2 && state.Prefix != hex.EncodeToString(prefix.Sum(nil)) {
					return Page{}, cursorError{}
				}
				resumed = true
			}
			if !requested[hit.Type] || strings.TrimSpace(hit.LUID) == "" || strings.TrimSpace(hit.Name) == "" {
				return Page{}, errors.New("native search returned invalid authoritative content identity")
			}
			hitKey := hit.Type + "\x00" + hit.LUID
			if seenHits[hitKey] && !result.UnresolvedMoreAvailable {
				result.UnresolvedMoreAvailable = true
				result.MoreAvailable = true
				result.Warnings = append(result.Warnings, "Tableau repeated a search hit across pages; first occurrences are retained, but completeness could not be established. Narrow the search or increase --limit to reduce pagination.")
			}
			seenHits[hitKey] = true
			item, include, err := a.contentItem(hit)
			if err != nil {
				return Page{}, err
			}
			if include {
				key := item.Type + "\x00" + item.LUID
				previous, duplicate := seen[key]
				if duplicate && (previous.Name != item.Name || previous.ProjectPath != item.ProjectPath) {
					return Page{}, errors.New("native search returned conflicting metadata for an authoritative content identity")
				}
				if !duplicate {
					if position >= resumeAt {
						// One unique lookahead distinguishes further content from
						// trailing duplicates or excluded embedded connections.
						if len(result.Items) == input.Limit {
							result.NextCursor = encodeNativeCursor(nativeCursor{Version: 2, Fingerprint: state.Fingerprint, Page: pageIndex, Offset: offset, Prefix: hex.EncodeToString(prefix.Sum(nil))})
							result.MoreAvailable = true
							if !result.UnresolvedMoreAvailable && !page.HasNext && offset == len(page.Items)-1 && total <= maxNativeHits {
								result.Total = len(seen) + 1
							} else if !result.UnresolvedMoreAvailable && !requested["datasource"] {
								result.Total = total
							}
							return result, nil
						}
						result.Items = append(result.Items, item)
					}
					seen[key] = item
				}
			}
			encoded, _ := json.Marshal(struct {
				Hit     tableausearch.Item
				Content Item
			}{hit, item})
			_, _ = prefix.Write(encoded)
			_, _ = prefix.Write([]byte{'\n'})
		}
		if !page.HasNext {
			break
		}
	}
	if !resumed {
		return Page{}, cursorError{}
	}
	if !result.MoreAvailable {
		result.Total = len(seen)
	} else if !result.UnresolvedMoreAvailable && !requested["datasource"] {
		result.Total = total
	}
	return result, nil
}

func publishedDatasource(hit tableausearch.Item) bool {
	return hit.Type == "datasource" && (hit.DatasourceIsPublished == nil || *hit.DatasourceIsPublished) &&
		(hit.ParentType == "" || strings.EqualFold(hit.ParentType, "Datasource"))
}

func (a *NativeAdapter) nativePage(ctx context.Context, input Input, index int) (tableausearch.Page, error) {
	if page, ok := a.pages[index]; ok {
		return page, nil
	}
	page, err := a.client.Search(ctx, tableauSearchRequest(input, index))
	if err != nil {
		return tableausearch.Page{}, err
	}
	if page.PageIndex != index || page.Limit != input.Limit || len(page.Items) > input.Limit || page.Total < index*input.Limit+len(page.Items) || (page.HasNext && len(page.Items) == 0) {
		return tableausearch.Page{}, errors.New("native search response changed the requested page")
	}
	urls := []string{}
	requested := make(map[string]bool)
	seen := make(map[string]bool)
	for _, hit := range page.Items {
		key := hit.Type + "\x00" + hit.LUID
		if seen[key] {
			return tableausearch.Page{}, errors.New("native search returned a duplicate hit within one page")
		}
		seen[key] = true
		if !publishedDatasource(hit) {
			continue
		}
		if hit.ContentURL == "" || a.datasourceResolver == nil {
			return tableausearch.Page{}, errors.New("native datasource search result cannot be mapped to a classic REST LUID")
		}
		if _, resolved := a.datasourceIDs[hit.ContentURL]; !resolved && !requested[hit.ContentURL] {
			requested[hit.ContentURL] = true
			urls = append(urls, hit.ContentURL)
		}
	}
	if len(urls) != 0 {
		resolved, err := a.datasourceResolver.ResolveContentURLs(ctx, urls)
		if err != nil {
			return tableausearch.Page{}, err
		}
		for _, url := range urls {
			id := strings.TrimSpace(resolved[url])
			if id == "" {
				return tableausearch.Page{}, errors.New("native datasource search result has no classic REST LUID")
			}
			a.datasourceIDs[url] = id
		}
	}
	a.pages[index] = page
	return page, nil
}

func (a *NativeAdapter) contentItem(hit tableausearch.Item) (Item, bool, error) {
	id, name, modified := hit.LUID, hit.Name, hit.ModifiedAt
	if hit.Type == "datasource" {
		if !publishedDatasource(hit) {
			return Item{}, false, nil
		}
		id = a.datasourceIDs[hit.ContentURL]
		if id == "" || (hit.DatasourceLUID != "" && hit.DatasourceLUID != id) || (hit.ParentLUID != "" && hit.ParentLUID != id) {
			return Item{}, false, errors.New("native datasource search parent disagrees with the classic REST LUID")
		}
		name = cmp.Or(hit.ParentName, name)
		modified = cmp.Or(hit.DatasourceUpdatedAt, modified)
	}
	return Item{LUID: id, Type: hit.Type, Name: name, ProjectPath: hit.ProjectPath,
		Owner: cmp.Or(hit.OwnerName, hit.OwnerLUID), ModifiedAt: modified}, true, nil
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
