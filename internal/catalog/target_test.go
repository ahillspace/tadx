package catalog

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTargetStoresSeparateOriginsSitesAndLegacy(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	first := NewTargetStore(root, "https://ONE.example:443/", "site", nil)
	equivalent := NewTargetStore(root, "https://one.example", "site", nil)
	second := NewTargetStore(root, "https://two.example", "site", nil)
	otherSite := NewTargetStore(root, "https://one.example", "other", nil)
	legacy := NewStore(root, nil)
	if first.RelativePath() != equivalent.RelativePath() {
		t.Fatal("equivalent origins differ")
	}
	base := NewTargetStore(root, "https://one.example/tableau/", "site", nil)
	baseEquivalent := NewTargetStore(root, "https://ONE.example:443/tableau", "site", nil)
	otherBase := NewTargetStore(root, "https://one.example/another", "site", nil)
	if base.RelativePath() != baseEquivalent.RelativePath() || base.RelativePath() == first.RelativePath() || base.RelativePath() == otherBase.RelativePath() {
		t.Fatal("reverse-proxy base path identity mismatch")
	}
	entry := ResourceEntry{Environment: "production", Site: "site", Kind: "datasource_schema", LUID: "same-id", Name: "First", Coverage: "detail", ObservedAt: now}
	if err := first.UpsertResources(context.Background(), []ResourceEntry{entry}); err != nil {
		t.Fatal(err)
	}
	query := ResourceQuery{Environment: "production", Site: "site", Kind: "datasource_schema", Limit: 10000}
	if result, err := equivalent.ReadResources(context.Background(), query); err != nil || len(result.Entries) != 1 {
		t.Fatalf("normalized target: %#v %v", result, err)
	}
	for _, store := range []*Store{second, otherSite, legacy} {
		_, err := store.ReadResources(context.Background(), query)
		var missing interface{ CatalogUninitialized() bool }
		if !errors.As(err, &missing) {
			t.Fatalf("another namespace returned %v", err)
		}
	}
}

func TestDeferredRefreshPublishesOnlyToCapturedTarget(t *testing.T) {
	root := t.TempDir()
	first := NewTargetStore(root, "https://one.example", "site", nil)
	second := NewTargetStore(root, "https://two.example", "site", nil)
	metadata := GenerationMetadata{Environment: "production", Site: "site", GeneratedAt: time.Now(), RequestedScopes: []string{"workbooks"}}
	writer, err := first.BeginRefreshGeneration(context.Background(), metadata)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.writeRecords(context.Background(), []Record{{LUID: "old-command", Kind: "workbook", Name: "First"}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.CompleteScopes(context.Background(), []string{"workbooks"}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Replace(context.Background(), Generation{Environment: "production", Site: "site", GeneratedAt: time.Now(), Complete: true, Scopes: []string{"workbooks"}, Records: []Record{{LUID: "new-command", Kind: "workbook", Name: "Second"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		store *Store
		id    string
	}{{first, "old-command"}, {second, "new-command"}} {
		result, err := test.store.Search(context.Background(), Query{Environment: "production", Site: "site", SiteSelected: true, Limit: 10000})
		if err != nil || len(result.Records) != 1 || result.Records[0].LUID != test.id {
			t.Fatalf("captured target: %#v %v", result, err)
		}
	}
}

func TestInvalidTargetCannotOpenLegacyStore(t *testing.T) {
	for _, server := range []string{"", "https://user:secret@example.com", "https://example.com?query", "file:///tmp"} {
		store := NewTargetStore(t.TempDir(), server, "", nil)
		if _, err := store.Status(context.Background(), Selection{Environment: "production", SiteSelected: true}); err == nil {
			t.Fatalf("accepted invalid target %q", server)
		}
	}
}
