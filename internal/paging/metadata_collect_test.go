package paging

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type metadataRow struct {
	ID   string
	Name string
}

func TestCollectMetadataRetainsPartialEvidence(t *testing.T) {
	lateFailure := errors.New("later page failed")
	tests := []struct {
		name      string
		pages     []value.MetadataPage[metadataRow]
		readError error
		wantError string
		wantItems int
		wantTotal int
		wantStamp string
		wantID    string
	}{
		{"later read", []value.MetadataPage[metadataRow]{{Items: []metadataRow{{"one", "One"}}, Total: 2, NextCursor: "next", ObservedAt: "first", TableauRequestID: "request-1"}}, lateFailure, "later page failed", 1, 2, "first", "request-1"},
		{"conflicting duplicate", []value.MetadataPage[metadataRow]{{Items: []metadataRow{{"one", "One"}}, Total: 2, NextCursor: "next", ObservedAt: "first"}, {Items: []metadataRow{{"one", "Changed"}}, Total: 2, ObservedAt: "second", TableauRequestID: "request-2"}}, nil, "conflicting duplicate metadata identity", 1, 2, "second", "request-2"},
		{"missing identity", []value.MetadataPage[metadataRow]{{Items: []metadataRow{{"one", "One"}, {" ", "Missing"}}, Total: 2, Complete: true, ObservedAt: "first"}}, nil, "metadata item has no authoritative identity", 1, 0, "first", ""},
		{"terminal coverage", []value.MetadataPage[metadataRow]{{Items: []metadataRow{{"one", "One"}}, Total: 2, Complete: true, ObservedAt: "first"}}, nil, "terminal coverage incomplete", 1, 2, "first", ""},
		{"repeated cursor", []value.MetadataPage[metadataRow]{{Items: []metadataRow{{"one", "One"}}, Total: 2, NextCursor: "next"}, {Items: []metadataRow{{"one", "One"}}, Total: 2, NextCursor: "next"}}, nil, "repeated metadata continuation", 1, 2, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			result, err := CollectMetadata(t.Context(), 25, func(context.Context, int, string) (value.MetadataPage[metadataRow], error) {
				defer func() { calls++ }()
				if calls == len(tt.pages) {
					return value.MetadataPage[metadataRow]{}, tt.readError
				}
				return tt.pages[calls], nil
			}, func(row metadataRow) string { return row.ID })
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
			if len(result.Items) != tt.wantItems || result.Total != tt.wantTotal || result.ObservedAt != tt.wantStamp || result.TableauRequestID != tt.wantID {
				t.Fatalf("result = %+v", result)
			}
		})
	}
}

func TestCollectMetadataDeduplicates(t *testing.T) {
	pages := []value.MetadataPage[metadataRow]{
		{Items: []metadataRow{{"one", "One"}}, Total: 2, NextCursor: "next"},
		{Items: []metadataRow{{"one", "One"}, {"two", "Two"}}, Total: 2, Complete: true},
	}
	calls := 0
	result, err := CollectMetadata(t.Context(), 3, func(_ context.Context, size int, cursor string) (value.MetadataPage[metadataRow], error) {
		if calls == 0 && (size != 3 || cursor != "") || calls == 1 && (size != 2 || cursor != "next") {
			t.Fatalf("request %d: size %d, cursor %q", calls, size, cursor)
		}
		page := pages[calls]
		calls++
		return page, nil
	}, func(row metadataRow) string { return row.ID })
	if err != nil || len(result.Items) != 2 || !result.Complete || result.MoreAvailable || calls != 2 {
		t.Fatalf("result = %+v, calls = %d, error = %v", result, calls, err)
	}
}

func TestCollectMetadataStopsAtLimit(t *testing.T) {
	calls := 0
	result, err := CollectMetadata(t.Context(), 2, func(_ context.Context, size int, cursor string) (value.MetadataPage[metadataRow], error) {
		calls++
		if size != 2 || cursor != "" {
			t.Fatalf("size %d, cursor %q", size, cursor)
		}
		return value.MetadataPage[metadataRow]{Items: []metadataRow{{"one", "One"}, {"two", "Two"}}, Total: 3, NextCursor: "later"}, nil
	}, func(row metadataRow) string { return row.ID })
	if err != nil || len(result.Items) != 2 || result.Complete || !result.MoreAvailable || calls != 1 {
		t.Fatalf("result = %+v, calls = %d, error = %v", result, calls, err)
	}
}
