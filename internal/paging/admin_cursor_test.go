package paging

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAdminCursorWireCompatibility(t *testing.T) {
	const payload = `{"Version":1,"Page":2,"Size":25,"Filter":"filter","Snapshot":"opaque"}`
	encoded, err := EncodeAdminCursor(2, 25, "filter", "opaque")
	if err != nil || encoded != base64.RawURLEncoding.EncodeToString([]byte(payload)) {
		t.Fatalf("encoded=%q err=%v", encoded, err)
	}
	cursor, valid := DecodeAdminCursor(encoded)
	if !valid || cursor != (AdminCursor{Version: 1, Page: 2, Size: 25, Filter: "filter", Snapshot: "opaque"}) {
		t.Fatalf("cursor=%+v valid=%v", cursor, valid)
	}
	for _, raw := range []string{
		`{}`,
		strings.Replace(payload, `"Version":1`, `"Version":2`, 1),
		strings.Replace(payload, `"Page":2`, `"Page":1`, 1),
		strings.Replace(payload, `"Size":25`, `"Size":0`, 1),
		strings.Replace(payload, `"Size":25`, `"Size":101`, 1),
		strings.Replace(payload, "opaque", strings.Repeat("x", 1025), 1),
	} {
		if _, valid := DecodeAdminCursor(base64.RawURLEncoding.EncodeToString([]byte(raw))); valid {
			t.Fatalf("accepted %q", raw)
		}
	}
	// Older cursors need not contain Snapshot, and unknown fields remain tolerated.
	for _, raw := range []string{`{"Version":1,"Page":2,"Size":100,"Filter":""}`, strings.Replace(payload, `"Version":1`, `"Extra":true,"Version":1`, 1)} {
		if _, valid := DecodeAdminCursor(base64.RawURLEncoding.EncodeToString([]byte(raw))); !valid {
			t.Fatalf("rejected %q", raw)
		}
	}
	if _, valid := DecodeAdminCursor(strings.Repeat("A", 2049)); valid {
		t.Fatal("accepted oversized token")
	}
	if _, valid := DecodeAdminCursor("!"); valid {
		t.Fatal("accepted invalid encoding")
	}
}
