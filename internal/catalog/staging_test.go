package catalog

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestRefreshStageCleanupAndAtomicFailure(t *testing.T) {
	for _, outcome := range []string{"success", "canceled", "incomplete", "publication_failure"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			store := NewStore(t.TempDir(), func() time.Time { return now })
			if _, err := store.Replace(ctx, Generation{ID: "prior", Environment: "dev", Site: "site", GeneratedAt: now, Complete: true, Source: "tableau-rest", Scopes: []string{"projects"}, Records: []Record{{LUID: "old", Kind: "project", Name: "Old"}}}); err != nil {
				t.Fatal(err)
			}
			writer, err := store.BeginRefreshGeneration(ctx, GenerationMetadata{ID: "next", Environment: "dev", Site: "site", GeneratedAt: now.Add(time.Hour), Source: "tableau-rest", RequestedScopes: []string{"projects"}})
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback()
			directory := writer.stagingDirectory
			if directory == "" {
				t.Fatal("refresh has no private staging directory")
			}
			var maximum int
			if err := writer.tx.QueryRowContext(ctx, "PRAGMA max_page_count").Scan(&maximum); err != nil || maximum != maximumStagePages {
				t.Fatalf("staging page cap %d: %v", maximum, err)
			}
			if err := writer.WriteBatch(ctx, Batch{Scope: "projects", Columns: batchColumns["projects"], Rows: [][]any{{"new", "New", "", "", "", `{}`}}}); err != nil {
				t.Fatal(err)
			}
			if outcome != "incomplete" {
				if err := writer.CompleteScopes(ctx, []string{"projects"}); err != nil {
					t.Fatal(err)
				}
			}
			if outcome == "publication_failure" {
				db, err := store.open(ctx)
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.ExecContext(ctx, `CREATE TRIGGER reject_snapshot BEFORE INSERT ON resource_scope_snapshots BEGIN SELECT RAISE(ABORT,'forced publication failure'); END`)
				db.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			publishContext, cancel := context.WithCancel(ctx)
			if outcome == "canceled" {
				cancel()
			}
			_, err = writer.Publish(publishContext)
			cancel()
			if outcome == "success" && err != nil {
				t.Fatal(err)
			}
			if outcome != "success" && err == nil {
				t.Fatal("failed refresh unexpectedly published")
			}
			if err := writer.Rollback(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stage was not removed: %v", err)
			}
			status, err := store.Status(ctx, Selection{Environment: "dev", Site: "site", SiteSelected: true})
			if err != nil {
				t.Fatal(err)
			}
			expected := "prior"
			if outcome == "success" {
				expected = "next"
			}
			if status.GenerationID != expected {
				t.Fatalf("generation=%s want=%s", status.GenerationID, expected)
			}
			rows, err := store.ReadResources(ctx, ResourceQuery{Environment: "dev", Site: "site", Kind: "project", Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			expectedID := "old"
			if outcome == "success" {
				expectedID = "new"
			}
			if len(rows.Entries) != 1 || rows.Entries[0].LUID != expectedID {
				t.Fatalf("partially published rows: %#v", rows)
			}
		})
	}
}

func TestRefreshStagingRejectsUnboundedRowsBeforeWriting(t *testing.T) {
	writer, err := NewStore(t.TempDir(), time.Now).BeginRefreshGeneration(context.Background(), GenerationMetadata{Environment: "dev", GeneratedAt: time.Now(), Source: "tableau-rest", RequestedScopes: []string{"projects"}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	writer.stagedRows = maximumResourceScopeRows
	if err := writer.WriteBatch(context.Background(), Batch{Scope: "projects", Columns: batchColumns["projects"], Rows: [][]any{{"new", "New", "", "", "", `{}`}}}); err == nil {
		t.Fatal("unbounded staged rows accepted")
	}
}
