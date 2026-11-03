package cache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkResourcePagination(b *testing.B) {
	store := NewStore(b.TempDir(), nil)
	entries := make([]ResourceEntry, 10000)
	for index := range entries {
		entries[index] = ResourceEntry{LUID: fmt.Sprintf("workbook-%05d", index), Name: fmt.Sprintf("Workbook %05d", index)}
	}
	if _, err := store.ReplaceResourceScope(b.Context(), ResourceScopeReplacement{Environment: "fixture", Site: "fixture", Kind: "workbook", Source: "tableau-rest", GeneratedAt: time.Now(), Entries: entries}); err != nil {
		b.Fatal(err)
	}
	for _, useSession := range []bool{false, true} {
		name := "one-shot"
		if useSession {
			name = "session"
		}
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				var reader interface {
					ReadResources(context.Context, ResourceQuery) (ResourceResult, error)
				} = store
				var session *ResourceReadSession
				if useSession {
					var err error
					session, err = store.OpenResourceRead(b.Context(), "fixture", "fixture")
					if err != nil {
						b.Fatal(err)
					}
					reader = session
				}
				for offset := 0; offset < len(entries); offset += 100 {
					result, err := reader.ReadResources(b.Context(), ResourceQuery{Environment: "fixture", Site: "fixture", Kind: "workbook", Offset: offset, Limit: 100})
					if err != nil || len(result.Entries) != 100 {
						b.Fatalf("page at %d = %d entries, %v", offset, len(result.Entries), err)
					}
				}
				if session != nil {
					if err := session.Close(); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
