package toon_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/toon"
)

func FuzzEncodeDecodeStrings(f *testing.F) {
	for _, seed := range []string{"safe", "", "42", "true", "a:b", "#comment", "- item", "a,b", "a|b", "你好 👋", "line\nvalue", "a\\b\"c"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if !utf8.ValidString(value) {
			t.Skip()
		}
		encoded, err := toon.Encode(map[string]any{"value": value})
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := toon.Decode(encoded)
		if err != nil {
			t.Fatalf("decode %q: %v", encoded, err)
		}
		object, ok := decoded.(map[string]any)
		if !ok || object["value"] != value {
			t.Fatalf("round trip mismatch: %#v", decoded)
		}
	})
}

func FuzzDecodeNeverPanics(f *testing.F) {
	for _, seed := range [][]byte{nil, []byte("a: 1"), []byte("x[2]: a,b"), []byte("a: \\\"bad\\q\\\""), {0xff, 0xfe}} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = toon.Decode(input)
	})
}

func FuzzJSONRoundTrip(f *testing.F) {
	for _, seed := range []string{`null`, `true`, `42`, `"x"`, `[]`, `{}`, `{"a":[1,"2",null]}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if !json.Valid([]byte(input)) {
			t.Skip()
		}
		var value any
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Skip()
		}
		encoded, err := toon.Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := toon.Decode(encoded)
		if err != nil {
			t.Fatalf("decode encoded JSON: %v\n%s", err, encoded)
		}
		want, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		got, err := json.Marshal(decoded)
		if err != nil {
			t.Fatal(err)
		}
		var wantValue any
		var gotValue any
		if err := json.Unmarshal(want, &wantValue); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(got, &gotValue); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotValue, wantValue) {
			t.Fatalf("JSON round trip mismatch\nwant: %s\ngot:  %s\nTOON:\n%s", want, got, encoded)
		}
		encodedAgain, err := toon.Encode(decoded)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encodedAgain, encoded) {
			t.Fatalf("encoding is not deterministic\nfirst:\n%s\nsecond:\n%s", encoded, encodedAgain)
		}
	})
}
