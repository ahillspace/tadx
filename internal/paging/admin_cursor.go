package paging

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

// AdminCursor preserves the version-one inventory envelope, including its
// original capitalized JSON field names and opaque cache snapshot token.
type AdminCursor struct {
	Version, Page, Size int
	Filter              string
	Snapshot            string
}

func DecodeAdminCursor(encoded string) (AdminCursor, bool) {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	var cursor AdminCursor
	valid := len(encoded) <= 2048 && err == nil && json.Unmarshal(data, &cursor) == nil &&
		cursor.Version == 1 && cursor.Page >= 2 && cursor.Size >= 1 && cursor.Size <= 100 && len(cursor.Snapshot) <= 1024
	return cursor, valid
}

func EncodeAdminCursor(page, size int, filter, snapshot string) (string, error) {
	data, err := json.Marshal(AdminCursor{Version: 1, Page: page, Size: size, Filter: filter, Snapshot: snapshot})
	return base64.RawURLEncoding.EncodeToString(data), err
}

// AdminCursorFingerprint hashes the resource-owned ordered filter record.
func AdminCursorFingerprint(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
