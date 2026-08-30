package toon_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/toon"
)

type cycleMarshaler struct {
	self *cycleMarshaler
}

func (value *cycleMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`{"value":"custom"}`), nil
}

type pointerReceiverMarshaler struct {
	Internal string `json:"internal"`
}

func (*pointerReceiverMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`"custom"`), nil
}

type pointerReceiverTextMarshaler struct {
	Internal string `json:"internal"`
}

func (*pointerReceiverTextMarshaler) MarshalText() ([]byte, error) {
	return []byte("custom text"), nil
}

type cycleTextMarshaler struct {
	Self *cycleTextMarshaler `json:"self"`
}

func (*cycleTextMarshaler) MarshalText() ([]byte, error) {
	return []byte("custom"), nil
}

type EmbeddedConflictLeft struct {
	Value string
}

type EmbeddedConflictRight struct {
	Value string
}

type jsonMarshalerMapKey string

func (jsonMarshalerMapKey) MarshalJSON() ([]byte, error) {
	return []byte(`"valid"`), nil
}

type textMarshalerMapKey string

func (textMarshalerMapKey) MarshalText() ([]byte, error) {
	return []byte("valid"), nil
}

func TestEncodeV41Conformance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		want string
		opts toon.EncodeOptions
	}{
		{name: "root object", json: `{"name":"Ada","active":true}`, want: "active: true\nname: Ada"},
		{name: "quoted values", json: `{"empty":"","number":"05","hash":"#tag","colon":"a:b","unicode":"你好 👋"}`, want: "colon: \"a:b\"\nempty: \"\"\nhash: \"#tag\"\nnumber: \"05\"\nunicode: 你好 👋"},
		{name: "escapes", json: `{"value":"a\\b\n\t\r\u0004\""}`, want: "value: \"a\\\\b\\n\\t\\r\\u0004\\\"\""},
		{name: "keys", json: `{"safe.key":1,"my-key":2,"2key":3}`, want: "\"2key\": 3\n\"my-key\": 2\nsafe.key: 1"},
		{name: "inline array", json: `{"tags":["admin","ops","dev"]}`, want: "tags[3]: admin,ops,dev"},
		{name: "empty arrays", json: `{"tags":[]}`, want: "tags: []"},
		{name: "root empty array", json: `[]`, want: "[]"},
		{name: "tabular array", json: `{"users":[{"id":1,"name":"Ada"},{"name":"Bob","id":2}]}`, want: "users[2]{id,name}:\n  1,Ada\n  2,Bob"},
		{name: "nested field group", json: `{"users":[{"id":1,"customer":{"name":"Ada","country":"UK"}},{"customer":{"country":"US","name":"Bob"},"id":2}]}`, want: "users[2]{customer{country,name},id}:\n  UK,Ada,1\n  US,Bob,2"},
		{name: "keyed table", json: `{"users":{"alice":{"id":1,"role":"admin"},"bob":{"role":"user","id":2}}}`, want: "users[2:]{id,role}:\n  alice: 1,admin\n  bob: 2,user"},
		{name: "root keyed table", json: `{"alice":{"id":1},"bob":{"id":2}}`, want: "[2:]{id}:\n  alice: 1\n  bob: 2"},
		{name: "nonuniform list", json: `{"items":[{"id":1,"name":"Ada"},{"id":2,"active":true}]}`, want: "items[2]:\n  - id: 1\n    name: Ada\n  - active: true\n    id: 2"},
		{name: "nested primitive arrays", json: `{"matrix":[[1,2],[3,4]]}`, want: "matrix[2]:\n  - [2]: 1,2\n  - [2]: 3,4"},
		{name: "pipe", json: `{"tags":["a,b","x|y"]}`, want: "tags[2|]: a,b|\"x|y\"", opts: toon.EncodeOptions{Delimiter: '|'}},
		{name: "four space indent", json: `{"a":{"b":{"c":1}}}`, want: "a:\n    b:\n        c: 1", opts: toon.EncodeOptions{IndentSize: 4}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var value any
			decoder := json.NewDecoder(strings.NewReader(tt.json))
			decoder.UseNumber()
			if err := decoder.Decode(&value); err != nil {
				t.Fatal(err)
			}
			got, err := toon.EncodeWithOptions(value, tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("encode mismatch\nwant:\n%s\ngot:\n%s", tt.want, got)
			}
		})
	}
}

func TestEncodeNormalizesNonFiniteNumbers(t *testing.T) {
	t.Parallel()

	got, err := toon.Encode(struct {
		NaN  float64 `json:"nan"`
		Neg  float64 `json:"neg"`
		Pos  float64 `json:"pos"`
		Zero float64 `json:"zero"`
	}{NaN: math.NaN(), Neg: math.Inf(-1), Pos: math.Inf(1), Zero: math.Copysign(0, -1)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "nan: null\nneg: null\npos: null\nzero: 0" {
		t.Fatalf("unexpected normalization:\n%s", got)
	}
}

func TestNumberDomainAndInvalidUTF8(t *testing.T) {
	t.Parallel()

	got, err := toon.Encode(json.Number("1e999999999999999999999"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1e+999999999999999999999" {
		t.Fatalf("unexpected large exponent: %s", got)
	}
	if _, err := toon.Encode(string([]byte{0xff})); err == nil {
		t.Fatal("expected invalid UTF-8 error")
	}
	if _, err := toon.Decode([]byte{0xff}); err == nil {
		t.Fatal("expected invalid UTF-8 decode error")
	}
}

func TestDecodeV41Conformance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty document", input: "", want: `{}`},
		{name: "comments and CRLF", input: "# heading\r\nname: Ada\r\n  # ignored\r\nactive: true\r\n", want: `{"name":"Ada","active":true}`},
		{name: "root primitive", input: `"42"`, want: `"42"`},
		{name: "numeric input", input: "a: 1.5000\nb: -1E+03\nc: 05\nd: +5", want: `{"a":1.5,"b":-1000,"c":"05","d":"+5"}`},
		{name: "legacy empty arrays", input: "a[0]:\nb: []", want: `{"a":[],"b":[]}`},
		{name: "inline empty cells", input: "a[3]: x,,z", want: `{"a":["x","","z"]}`},
		{name: "tabular nested fields", input: "users[2]{id,customer{name,country}}:\n  1,Ada,UK\n  2,Bob,US", want: `{"users":[{"id":1,"customer":{"name":"Ada","country":"UK"}},{"id":2,"customer":{"name":"Bob","country":"US"}}]}`},
		{name: "keyed root", input: "[2:]{id,role}:\n  alice: 1,admin\n  bob: 2,user", want: `{"alice":{"id":1,"role":"admin"},"bob":{"id":2,"role":"user"}}`},
		{name: "mixed list", input: "items[3]:\n  - one\n  - id: 2\n    ok: true\n  - [2|]: x|y", want: `{"items":["one",{"id":2,"ok":true},["x","y"]]}`},
		{name: "quoted escapes", input: `value: "a\\b\n\t\r\u0004\""`, want: `{"value":"a\\b\n\t\r\u0004\""}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := toon.Decode([]byte(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			assertJSONEqual(t, tt.want, got)
		})
	}
}

func TestDecodeStrictErrors(t *testing.T) {
	t.Parallel()

	inputs := []string{
		"tags[2]: one",
		"items[1]{a,b}:\n  1",
		"items[1]:\n   - x",
		"a: 1\n  b: 2",
		"a: 1\na: 2",
		`a: "bad\q"`,
		"hello\nworld",
		"[1]: x\ntrailing: true",
	}
	for _, input := range inputs {
		if _, err := toon.Decode([]byte(input)); err == nil {
			t.Errorf("expected strict error for %q", input)
		}
	}
}

func TestDecodeNonStrictCountAndDuplicateHandling(t *testing.T) {
	t.Parallel()

	got, err := toon.DecodeWithOptions([]byte("a[1]: x,y\nk: 1\nk: 2"), toon.DecodeOptions{IndentSize: 2, Strict: false})
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, `{"a":["x","y"],"k":2}`, got)
}

func TestRoundTripJSONModel(t *testing.T) {
	t.Parallel()

	input := `{"z":null,"s":"05","n":1.25,"b":true,"a":[1,"x",false,null,{"nested":[{"id":1},{"id":2}]}],"o":{"x":1,"y":[]}}`
	var value any
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := toon.Decode(encoded)
	if err != nil {
		t.Fatalf("decode encoded value: %v\n%s", err, encoded)
	}
	assertJSONEqual(t, input, decoded)
	encodedAgain, err := toon.Encode(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(encoded, encodedAgain) {
		t.Fatalf("encoding is not deterministic\nfirst:\n%s\nsecond:\n%s", encoded, encodedAgain)
	}
}

func TestDecodeBoundsCapacityFromDeclaredCollectionLength(t *testing.T) {
	t.Parallel()

	declared := strconv.Itoa(int(^uint(0) >> 1))
	for _, input := range []string{
		"items[" + declared + "]:",
		"items[" + declared + "]{value}:",
	} {
		if _, err := toon.Decode([]byte(input)); err == nil || !strings.Contains(err.Error(), "declares") {
			t.Errorf("Decode(%q) error = %v, want declared-count mismatch", input, err)
		}
	}
}

func TestEncodeAllowsSharedAcyclicReferences(t *testing.T) {
	t.Parallel()

	sharedMap := map[string]any{"value": "shared"}
	sharedSlice := []any{"shared"}
	value := map[string]any{
		"maps":   []any{sharedMap, sharedMap},
		"slices": []any{sharedSlice, sharedSlice},
	}

	if _, err := toon.Encode(value); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
}

func TestEncodeAllowsDistinctSliceViewsWithSharedBackingStorage(t *testing.T) {
	t.Parallel()

	outer := []any{"x", nil}
	outer[1] = outer[:1]
	value := struct {
		Views []any   `json:"views"`
		NaN   float64 `json:"nan"`
	}{Views: outer, NaN: math.NaN()}

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := toon.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	assertJSONEqual(t, `{"views":["x",["x"]],"nan":null}`, decoded)
}

func TestEncodeQuotesUnicodeEdgeWhitespace(t *testing.T) {
	t.Parallel()

	encoded, err := toon.Encode("\u00a0")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := toon.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode(%q) error = %v", encoded, err)
	}
	if decoded != "\u00a0" {
		t.Fatalf("round trip = %#v, want non-breaking space", decoded)
	}
}

func TestEncodePreservesSharedReferencesWhenNormalizingNonFiniteValues(t *testing.T) {
	t.Parallel()

	sharedMap := map[string]any{"value": math.NaN()}
	sharedSlice := []any{math.Inf(1)}
	encoded, err := toon.Encode(map[string]any{
		"maps":   []any{sharedMap, sharedMap},
		"slices": []any{sharedSlice, sharedSlice},
	})
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := toon.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	assertJSONEqual(t, `{"maps":[{"value":null},{"value":null}],"slices":[[null],[null]]}`, decoded)
}

func TestEncodePreservesFiniteFloat32DuringNonFiniteFallback(t *testing.T) {
	t.Parallel()

	value := struct {
		Finite    float32 `json:"finite"`
		NonFinite float64 `json:"non_finite"`
	}{
		Finite:    1.2,
		NonFinite: math.NaN(),
	}

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), "finite: 1.2\nnon_finite: null"; got != want {
		t.Fatalf("Encode() = %q, want %q", got, want)
	}
}

func TestEncodeRejectsRealCycles(t *testing.T) {
	t.Parallel()

	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := make([]any, 1)
	cyclicSlice[0] = cyclicSlice

	for _, value := range []any{cyclicMap, cyclicSlice} {
		if _, err := toon.Encode(value); err == nil || !strings.Contains(err.Error(), "cyclic value") {
			t.Errorf("Encode(%T) error = %v, want cyclic-value error", value, err)
		}
	}
}

func TestEncodeIgnoresCycleInExcludedJSONField(t *testing.T) {
	t.Parallel()

	type valueWithExcludedCycle struct {
		Name string                  `json:"name"`
		Self *valueWithExcludedCycle `json:"-"`
	}

	value := &valueWithExcludedCycle{Name: "safe"}
	value.Self = value

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(encoded) != "name: safe" {
		t.Fatalf("Encode() = %q, want %q", encoded, "name: safe")
	}
}

func TestEncodeStopsValidationAtJSONMarshaler(t *testing.T) {
	t.Parallel()

	value := &cycleMarshaler{}
	value.self = value

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(encoded) != "value: custom" {
		t.Fatalf("Encode() = %q, want %q", encoded, "value: custom")
	}
}

func TestEncodePreservesPointerReceiverMarshalerDuringNonFiniteFallback(t *testing.T) {
	t.Parallel()

	value := &struct {
		Custom pointerReceiverMarshaler `json:"custom"`
		NaN    float64                  `json:"nan"`
	}{Custom: pointerReceiverMarshaler{Internal: "not custom"}, NaN: math.NaN()}

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(encoded) != "custom: custom\nnan: null" {
		t.Fatalf("Encode() = %q, want pointer-receiver output", encoded)
	}
}

func TestEncodePreservesPointerReceiverTextMarshalerDuringNonFiniteFallback(t *testing.T) {
	t.Parallel()

	value := &struct {
		Custom pointerReceiverTextMarshaler `json:"custom"`
		NaN    float64                      `json:"nan"`
	}{Custom: pointerReceiverTextMarshaler{Internal: "not custom"}, NaN: math.NaN()}

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(encoded) != "custom: custom text\nnan: null" {
		t.Fatalf("Encode() = %q, want pointer-receiver text output", encoded)
	}
}

func TestEncodeStopsValidationAtTextMarshaler(t *testing.T) {
	t.Parallel()

	value := &cycleTextMarshaler{}
	value.Self = value

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(encoded) != "custom" {
		t.Fatalf("Encode() = %q, want %q", encoded, "custom")
	}
}

func TestEncodeIgnoresConflictingEmbeddedJSONFields(t *testing.T) {
	t.Parallel()

	invalid := string([]byte{0xff})
	value := struct {
		EmbeddedConflictLeft
		EmbeddedConflictRight
		Safe string `json:"safe"`
	}{
		EmbeddedConflictLeft:  EmbeddedConflictLeft{Value: invalid},
		EmbeddedConflictRight: EmbeddedConflictRight{Value: invalid},
		Safe:                  "kept",
	}

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if string(encoded) != "safe: kept" {
		t.Fatalf("Encode() = %q, want %q", encoded, "safe: kept")
	}
}

func TestEncodeSelectsJSONFieldNamedDash(t *testing.T) {
	t.Parallel()

	value := struct {
		Dash string `json:"-,"`
	}{Dash: string([]byte{0xff})}

	if _, err := toon.Encode(value); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("Encode() error = %v, want invalid UTF-8 error", err)
	}
}

func TestEncodeValidatesNamedStringMapKeysBeforeMarshalerMethods(t *testing.T) {
	t.Parallel()

	invalid := string([]byte{0xff})
	values := []any{
		map[jsonMarshalerMapKey]string{jsonMarshalerMapKey(invalid): "value"},
		map[textMarshalerMapKey]string{textMarshalerMapKey(invalid): "value"},
	}

	for _, value := range values {
		if _, err := toon.Encode(value); err == nil || !strings.Contains(err.Error(), "valid UTF-8") {
			t.Errorf("Encode(%T) error = %v, want invalid UTF-8 error", value, err)
		}
	}
}

func TestEncodePreservesJSONStringTagDuringNonFiniteFallback(t *testing.T) {
	t.Parallel()

	value := struct {
		Count int     `json:"count,string"`
		NaN   float64 `json:"nan"`
	}{Count: 42, NaN: math.NaN()}

	encoded, err := toon.Encode(value)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := toon.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	assertJSONEqual(t, `{"count":"42","nan":null}`, decoded)
}

func TestSpecVersion(t *testing.T) {
	if toon.SpecVersion != "4.1" {
		t.Fatalf("unexpected spec version %q", toon.SpecVersion)
	}
}

func assertJSONEqual(t *testing.T, wantJSON string, got any) {
	t.Helper()
	var want any
	decoder := json.NewDecoder(strings.NewReader(wantJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&want); err != nil {
		t.Fatal(err)
	}
	wantBytes, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Fatalf("JSON mismatch\nwant: %s\ngot:  %s", wantBytes, gotBytes)
	}
}
