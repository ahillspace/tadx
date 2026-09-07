package catalog

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const partialInventoryCursorPrefix = "inventory:"
const partialInventoryLifetime = 24 * time.Hour
const maximumPartialInventories = 100
const partialInventoryDDL = `CREATE TABLE partial_inventories (id TEXT PRIMARY KEY,environment TEXT NOT NULL,site TEXT NOT NULL,kind TEXT NOT NULL,observed_at TEXT NOT NULL,expires_at TEXT NOT NULL,record_count INTEGER NOT NULL,warning TEXT NOT NULL) STRICT`
const partialInventoryRowsDDL = `CREATE TABLE partial_inventory_rows (snapshot_id TEXT NOT NULL REFERENCES partial_inventories(id) ON DELETE CASCADE,ordinal INTEGER NOT NULL,entry BLOB NOT NULL,PRIMARY KEY(snapshot_id,ordinal)) STRICT`

// SavePartialInventory preserves ordered valid rows for continuation only.
// These snapshots never participate in catalog search, inspection, or coverage.
// Snapshots expire after 24 hours; at most 100 snapshots are retained.
func (s *Store) SavePartialInventory(ctx context.Context, input ResourceScopeReplacement, warning string) (string, error) {
	if len(input.Entries) > maximumResourceScopeRows || input.Environment == "" || kindScope(input.Kind) == "" || input.GeneratedAt.IsZero() {
		return "", errors.New("invalid partial inventory scope")
	}
	identity := make([]byte, 16)
	if _, err := rand.Read(identity); err != nil {
		return "", err
	}
	id := hex.EncodeToString(identity)
	db, err := s.open(ctx)
	if err != nil {
		return "", err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM partial_inventories WHERE expires_at<=?`, s.now().UTC().Format(generationTimeLayout)); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM partial_inventories WHERE id IN (SELECT id FROM partial_inventories ORDER BY expires_at DESC,id DESC LIMIT -1 OFFSET ?)`, maximumPartialInventories-1); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO partial_inventories VALUES(?,?,?,?,?,?,?,?)`, id, input.Environment, input.Site, input.Kind, input.GeneratedAt.UTC().Format(generationTimeLayout), s.now().Add(partialInventoryLifetime).UTC().Format(generationTimeLayout), len(input.Entries), warning); err != nil {
		return "", err
	}
	statement, err := tx.PrepareContext(ctx, `INSERT INTO partial_inventory_rows VALUES(?,?,?)`)
	if err != nil {
		return "", err
	}
	defer statement.Close()
	for index, entry := range input.Entries {
		if entry.Environment != input.Environment || entry.Site != input.Site || entry.Kind != input.Kind || entry.LUID == "" || entry.Name == "" {
			return "", errors.New("partial inventory row does not match scope")
		}
		encoded, err := json.Marshal(entry)
		if err != nil {
			return "", err
		}
		if _, err := statement.ExecContext(ctx, id, index, encoded); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// PartialInventoryCursor binds an offset to one immutable temporary snapshot.
func PartialInventoryCursor(id string, query ResourceQuery) string {
	return partialInventoryCursorPrefix + encodeCursor(id, resourceQueryFingerprint(query), query.Offset)
}

func (s *Store) readPartialInventory(ctx context.Context, query ResourceQuery) (ResourceResult, error) {
	id, fingerprint, offset, err := decodeCursor(strings.TrimPrefix(query.Cursor, partialInventoryCursorPrefix))
	if err != nil || offset < 0 || query.Offset != 0 || fingerprint != resourceQueryFingerprint(query) || query.LUID != "" || query.Name != "" || query.ProjectPath != "" {
		return ResourceResult{}, invalidCursorError{}
	}
	db, err := s.open(ctx)
	if err != nil {
		return ResourceResult{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ResourceResult{}, err
	}
	defer tx.Rollback()
	var observed string
	result := ResourceResult{Coverage: "partial"}
	err = tx.QueryRowContext(ctx, `SELECT observed_at,record_count,warning FROM partial_inventories WHERE id=? AND environment=? AND site=? AND kind=? AND expires_at>?`, id, query.Environment, query.Site, query.Kind, s.now().UTC().Format(generationTimeLayout)).Scan(&observed, &result.Total, &result.InventoryWarning)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceResult{}, invalidCursorError{}
	}
	if err != nil {
		return ResourceResult{}, err
	}
	if offset >= result.Total {
		return ResourceResult{}, invalidCursorError{}
	}
	result.NewestObserved, err = time.Parse(generationTimeLayout, observed)
	if err != nil {
		return ResourceResult{}, err
	}
	result.Stale, _ = staleness(s.now, result.NewestObserved, id)
	rows, err := tx.QueryContext(ctx, `SELECT entry FROM partial_inventory_rows WHERE snapshot_id=? AND ordinal>=? ORDER BY ordinal LIMIT ?`, id, offset, query.Limit)
	if err != nil {
		return ResourceResult{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded []byte
		var entry ResourceEntry
		if err := rows.Scan(&encoded); err != nil {
			return ResourceResult{}, err
		}
		if err := json.Unmarshal(encoded, &entry); err != nil {
			return ResourceResult{}, err
		}
		result.Entries = append(result.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return ResourceResult{}, err
	}
	if len(result.Entries) != min(query.Limit, result.Total-offset) {
		return ResourceResult{}, invalidCursorError{}
	}
	if offset+len(result.Entries) < result.Total {
		query.Offset = offset + len(result.Entries)
		result.NextCursor = PartialInventoryCursor(id, query)
	}
	if err := tx.Commit(); err != nil {
		return ResourceResult{}, err
	}
	return result, nil
}
