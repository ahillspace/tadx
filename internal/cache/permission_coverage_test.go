package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/cache"
)

func TestPartialPermissionCoverageCannotBeConfusedWithEmptyPermissions(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := cache.NewStore(t.TempDir(), func() time.Time { return now })
	metadata := cache.GenerationMetadata{Environment: "test", Source: "tableau-rest", GeneratedAt: now, RequestedScopes: []string{"permissions"}, ImplicitScopes: []string{"projects", "workbooks"}}
	var completeID string
	for _, partial := range []bool{false, true} {
		writer, err := store.BeginGeneration(ctx, metadata)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Rollback()
		scopes := []string{"projects", "workbooks", "permissions"}
		if partial {
			scopes = scopes[:2]
			if err := writer.MarkPermissionsIncomplete(ctx); err != nil {
				t.Fatal(err)
			}
			if err := writer.CompleteScopes(ctx, []string{"permissions"}); err == nil {
				t.Fatal("denied permission coverage was marked complete")
			}
		}
		if err := writer.CompleteScopes(ctx, scopes); err != nil {
			t.Fatal(err)
		}
		published, err := writer.Publish(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !partial {
			completeID = published.GenerationID
		} else if published.GenerationID == completeID {
			t.Fatal("unknown permission coverage reused the complete empty generation identity")
		}
		status, err := store.Status(ctx, cache.Selection{Environment: "test", SiteSelected: true})
		if err != nil || status.Complete == partial {
			t.Fatalf("partial=%t status=%#v error=%v", partial, status, err)
		}
	}
}

func TestPartialPermissionsDoNotPermitIncompleteInventoryPublication(t *testing.T) {
	ctx := context.Background()
	store := cache.NewStore(t.TempDir(), nil)
	writer, err := store.BeginGeneration(ctx, cache.GenerationMetadata{Environment: "test", Source: "tableau-rest", GeneratedAt: time.Now(), RequestedScopes: []string{"workbooks", "permissions"}, ImplicitScopes: []string{"projects"}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.MarkPermissionsIncomplete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(ctx); err == nil {
		t.Fatal("permission denial bypassed incomplete workbook and project inventory")
	}
}
