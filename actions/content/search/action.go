// Package search defines the gated action seam for remote cross-resource content search.
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

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// Source searches current remote content through a resource adapter.
type Source interface {
	Search(context.Context, Input) (Result, error)
}

// Action orchestrates content.search without implementing an upstream protocol.
type Action struct{ source Source }

// New creates content.search.
func New(source Source) *Action { return &Action{source: source} }

// Execute validates filters and binds continuation state to the complete filter set.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, searchError("content.search.unconfigured", errs.KindRuntime, input, "Content search is not configured.", nil)
	}
	if input.Limit < 0 || input.Limit > maxLimit {
		return Output{}, searchError("content.search.usage", errs.KindUsage, input, "Content search limit must be nonnegative and at most 100.", nil)
	}
	if input.Environment != "" && !input.SiteResolved {
		return Output{}, searchError("content.search.usage", errs.KindUsage, input, "Content search requires a resolved source site.", nil)
	}
	request := input
	if request.Limit == 0 {
		request.Limit = defaultLimit
	}
	upstreamCursor, err := decodeCursor(request.Cursor, request)
	if err != nil {
		return Output{}, searchError("content.search.usage", errs.KindUsage, input, "Content search cursor does not match the selected filters.", err)
	}
	request.Cursor = upstreamCursor
	result, err := a.source.Search(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Output{}, searchError("content.search.cancelled", errs.KindOperation, input, "Content search was canceled.", err)
		}
		return Output{}, searchError("content.search.failed", errs.KindOperation, input, "Content search failed.", err)
	}
	if len(result.Items) > request.Limit {
		return Output{}, searchError("content.search.failed", errs.KindOperation, input, "Content search source exceeded the requested result bound.", nil)
	}
	// Preserve the upstream order verbatim: pagination continues on the upstream
	// cursor, so the adapter/upstream owns global ordering across pages. A local
	// per-page re-sort would break monotonic ordering across the cursor sequence.
	items := append([]Item{}, result.Items...)
	for index := range items {
		if strings.TrimSpace(items[index].LUID) == "" || strings.TrimSpace(items[index].Kind) == "" || strings.TrimSpace(items[index].Name) == "" {
			return Output{}, searchError("content.search.failed", errs.KindOperation, input, "Content search source returned an item without authoritative identity.", nil)
		}
	}
	page := result.Page
	page.Returned = len(items)
	page.Limit = request.Limit
	if page.NextCursor != "" {
		page.NextCursor = encodeCursor(page.NextCursor, request)
	}
	return Output{Page: page, Items: items, Warnings: output.BoundWarnings(result.Warnings), Help: []string{"tadx content get --kind <kind> --id <luid>"}}, nil
}

type cursorEnvelope struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	Cursor      string `json:"c"`
	Checksum    string `json:"s"`
}

func encodeCursor(upstream string, input Input) string {
	envelope := cursorEnvelope{Version: 1, Fingerprint: fingerprint(input), Cursor: upstream}
	envelope.Checksum = checksum(envelope.Version, envelope.Fingerprint, envelope.Cursor)
	data, _ := json.Marshal(envelope)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeCursor(value string, input Input) (string, error) {
	if value == "" {
		return "", nil
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	var envelope cursorEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return "", err
	}
	if envelope.Version != 1 || envelope.Cursor == "" || envelope.Fingerprint != fingerprint(input) || envelope.Checksum != checksum(envelope.Version, envelope.Fingerprint, envelope.Cursor) {
		return "", errors.New("invalid content search cursor")
	}
	return envelope.Cursor, nil
}

func fingerprint(input Input) string {
	kinds := append([]string(nil), input.Kinds...)
	sort.Strings(kinds)
	value := struct {
		Environment, Site, Terms, OwnerLUID, ProjectLUID, ModifiedAfter, ModifiedBefore string
		Kinds                                                                           []string
		Limit                                                                           int
	}{input.Environment, input.Site, input.Terms, input.OwnerLUID, input.ProjectLUID, input.ModifiedAfter, input.ModifiedBefore, kinds, input.Limit}
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func checksum(version int, fingerprint, cursor string) string {
	data, _ := json.Marshal([]any{version, fingerprint, cursor})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func searchError(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "content.search", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Adjust the bounded search filters, then retry."}
}
