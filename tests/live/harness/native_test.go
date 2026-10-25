package harness

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeNativeTestFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(filename, content, 0600); err != nil {
		t.Fatal(err)
	}
	return filename
}
func nativeZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range files {
		member, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := member.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func nativeSymlinkZIP(t *testing.T, name, target string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: name, Method: zip.Store}
	header.SetMode(os.ModeSymlink | 0600)
	member, err := archive.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte(target)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestNativeOracleRejectsBrokenOrUnsafePackages(t *testing.T) {
	for name, content := range map[string][]byte{
		"empty":              {},
		"wrong-root":         []byte(`<workbook/>`),
		"malformed":          []byte(`<datasource><column></datasource>`),
		"multiple-roots":     []byte(`<datasource/><datasource/>`),
		"missing-document":   nativeZIP(t, map[string]string{"data.txt": "not a source"}),
		"zip-traversal":      nativeZIP(t, map[string]string{"../fixture.tds": "<datasource/>"}),
		"duplicate-document": nativeZIP(t, map[string]string{"a.tds": "<datasource/>", "b.tds": "<datasource/>"}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateNative("datasource", Native{Bytes: content, Filename: "fixture.tdsx"}); err == nil {
				t.Fatal("invalid native artifact accepted")
			}
		})
	}
}

func TestNativeCompareIgnoresSerializationButDetectsChangedFormulaAndRows(t *testing.T) {
	left := nativeZIP(t, map[string]string{"fixture.tds": `<datasource name="same"><column name="[Sales]" datatype="real"><calculation formula="1+2"/></column></datasource>`, "Data/fixture.hyper": "extract-A"})
	right := nativeZIP(t, map[string]string{"fixture.tds": "<?xml version='1.0'?>\n<datasource name='same'>\n  <column datatype='real' name='[Sales]'><calculation formula='1+2'/></column>\n</datasource>", "Data/fixture.hyper": "extract-A"})
	if err := CompareNative("datasource", left, right); err != nil {
		t.Fatalf("serialization-only change failed: %v", err)
	}
	formula := nativeZIP(t, map[string]string{"fixture.tds": `<datasource name="same"><column name="[Sales]" datatype="real"><calculation formula="1+3"/></column></datasource>`, "Data/fixture.hyper": "extract-A"})
	if err := CompareNative("datasource", left, formula); err == nil {
		t.Fatal("changed formula accepted")
	}
	rows := nativeZIP(t, map[string]string{"fixture.tds": `<datasource name="same"><column name="[Sales]" datatype="real"><calculation formula="1+2"/></column></datasource>`, "Data/fixture.hyper": "extract-B"})
	if err := CompareNative("datasource", left, rows); err == nil {
		t.Fatal("changed extract accepted")
	}
}

func TestNativeManifestRejectsMissingAuthoredMarkers(t *testing.T) {
	data := []byte(`<workbook><datasources><datasource><column name="[Sales]"/></datasource></datasources><worksheets><worksheet name="Sales by region"/></worksheets></workbook>`)
	good := Binding{Path: "fixture.twb", Expected: map[string]any{"columns": []string{"[Sales]"}, "worksheets": []string{"Sales by region"}, "extract_count": 0}}
	if err := VerifyNative(t.Context(), "workbook", data, good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Expected = map[string]any{"columns": []string{"[Missing]"}}
	if err := VerifyNative(t.Context(), "workbook", data, bad); err == nil {
		t.Fatal("missing source field marker accepted")
	}
}

func TestNativeDatasourceRecognizesMetadataRecordColumns(t *testing.T) {
	data := nativeZIP(t, map[string]string{
		"fixture.tds":                 `<datasource><metadata-record class="column"><local-name>[Sales]</local-name></metadata-record><metadata-record class="column"><local-name>[Order Date]</local-name></metadata-record><metadata-record class="table"><local-name>[Not a column]</local-name></metadata-record></datasource>`,
		"Data/Extracts/fixture.hyper": "extract",
	})
	binding := Binding{Path: "fixture.tdsx", Expected: map[string]any{
		"columns":       []string{"[Sales]", "[Order Date]"},
		"extract_count": 1,
	}}
	if err := VerifyNative(t.Context(), "datasource", data, binding); err != nil {
		t.Fatalf("valid native metadata columns were not recognized: %v", err)
	}

	binding.Expected["columns"] = []string{"[Sales]", "[Missing]"}
	if err := VerifyNative(t.Context(), "datasource", data, binding); err == nil {
		t.Fatal("missing authored column was accepted")
	}
}

func TestRowOraclePreservesMultiplicityAndIgnoresOrder(t *testing.T) {
	a := []any{[]any{1, "East"}, []any{1, "East"}, []any{2, "West"}}
	b := []any{[]any{2, "West"}, []any{1, "East"}, []any{1, "East"}}
	if !sameRows(a, b) {
		t.Fatal("row order changed equality")
	}
	if sameRows(a, b[:2]) {
		t.Fatal("lost duplicate row accepted")
	}
	if sameRows(a, []any{[]any{1, "East"}, []any{2, "West"}, []any{2, "West"}}) {
		t.Fatal("different row multiset accepted")
	}
}

func TestNativeFlowRequiresAndVerifiesAuthoredSemantics(t *testing.T) {
	data := []byte(`{"nodes":{"input":{"name":"Fixture input"},"output":{"name":"Expected output","table":"Sales"}},"server_added":"volatile"}`)
	if err := VerifyNative(t.Context(), "flow", data, Binding{Path: "fixture.tfl"}); err == nil {
		t.Fatal("flow passed without authored semantic oracle")
	}
	good := Binding{Path: "fixture.tfl", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"name": "Expected output", "table": "Sales"}}}}}
	if err := VerifyNative(t.Context(), "flow", data, good); err != nil {
		t.Fatal(err)
	}
	bad := Binding{Path: "fixture.tfl", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"table": "Another table"}}}}}
	if err := VerifyNative(t.Context(), "flow", data, bad); err == nil {
		t.Fatal("wrong flow output table accepted")
	}
}

func TestNativeFlowTFLXPackageAndVariantExpectations(t *testing.T) {
	flowA := `{"initialNodes":["input"],"nodes":{"input":{"id":"input","name":"Fixture input","nodeType":".v1.LoadCsv","nextNodes":["output"]},"output":{"id":"output","name":"Sales output","nodeType":".v1.Output","nextNodes":[]}}}`
	flowB := `{"initialNodes":["input"],"nodes":{"input":{"id":"input","name":"Fixture input","nodeType":".v1.LoadCsv","nextNodes":["output"]},"output":{"id":"output","name":"Overwrite output","nodeType":".v1.Output","nextNodes":[]}}}`
	packageA := nativeZIP(t, map[string]string{"flow": flowA, "Data/input.csv": "value\n1\n"})
	packageB := nativeZIP(t, map[string]string{"flow": flowB, "Data/input.csv": "value\n1\n"})
	bindingA := Binding{Path: "fixture-a.tflx", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"nodeType": ".v1.Output", "name": "Sales output"}}}}}
	bindingB := Binding{Path: "fixture-b.tflx", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"nodeType": ".v1.Output", "name": "Overwrite output"}}}}}

	for name, test := range map[string]struct {
		data    []byte
		binding Binding
	}{
		"A matches A": {data: packageA, binding: bindingA},
		"B matches B": {data: packageB, binding: bindingB},
	} {
		t.Run(name, func(t *testing.T) {
			if err := VerifyNative(t.Context(), "flow", test.data, test.binding); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := VerifyNative(t.Context(), "flow", packageA, bindingB); err == nil {
		t.Fatal("flow A passed the distinct B expectation")
	}
	if err := VerifyNative(t.Context(), "flow", packageB, bindingA); err == nil {
		t.Fatal("flow B passed the distinct A expectation")
	}
}

func TestNativeFlowAcceptsRawJSONWithTFLXBindingPath(t *testing.T) {
	data := []byte(`{"nodes":{"output":{"name":"Expected output","nodeType":".v1.Output"}}}`)
	binding := Binding{Path: "fixture.tflx", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"name": "Expected output"}}}}}
	if err := VerifyNative(t.Context(), "flow", data, binding); err != nil {
		t.Fatal(err)
	}
}

func TestNativeFlowTFLXAcceptsTFLDocumentMember(t *testing.T) {
	data := nativeZIP(t, map[string]string{"document.tfl": `{"nodes":{"output":{"name":"Expected output"}}}`})
	binding := Binding{Path: "fixture.tflx", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"name": "Expected output"}}}}}
	if err := VerifyNative(t.Context(), "flow", data, binding); err != nil {
		t.Fatal(err)
	}
}

func TestNativeFlowTFLXRejectsMissingMalformedAmbiguousAndUnsafeDocuments(t *testing.T) {
	valid := `{"nodes":{"output":{"name":"Output"}}}`
	for name, data := range map[string][]byte{
		"missing document": nativeZIP(t, map[string]string{"Data/input.csv": "value\n1\n"}),
		"malformed flow":   nativeZIP(t, map[string]string{"flow": "not JSON"}),
		"ambiguous documents": nativeZIP(t, map[string]string{
			"flow":       valid,
			"nested.tfl": valid,
		}),
		"unsafe flow path":      nativeZIP(t, map[string]string{"../flow": valid}),
		"symlink flow document": nativeSymlinkZIP(t, "flow", valid),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ValidateNative("flow", Native{Bytes: data, Filename: "fixture.tflx"}); err == nil {
				t.Fatal("invalid flow package accepted")
			}
		})
	}
}

func TestNativeFlowMismatchReportsSafeJSONPath(t *testing.T) {
	data := []byte(`{"nodes":{"output":{"name":"actual-private-value"}}}`)
	binding := Binding{Path: "fixture.tfl", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"name": "expected-private-value", "table": "unseen-private-value"}}}}}
	err := VerifyNative(t.Context(), "flow", data, binding)
	if err == nil {
		t.Fatal("mismatched authored flow was accepted")
	}
	message := err.Error()
	for _, want := range []string{`$["nodes"]["output"]["name"]`, "string", "mismatch"} {
		if !strings.Contains(message, want) {
			t.Fatalf("mismatch error omitted safe path/type detail %q: %s", want, message)
		}
	}
	for _, private := range []string{"actual-private-value", "expected-private-value", "unseen-private-value"} {
		if strings.Contains(message, private) {
			t.Fatalf("mismatch error exposed authored scalar %q", private)
		}
	}
}

func TestNativeFlowSubsetAllowsExtraFieldsInsideArrayObjects(t *testing.T) {
	t.Run("server annotation is allowed", func(t *testing.T) {
		data := []byte(`{"nodes":{"node-1":{"fields":[{"name":"Sales","type":"number","isGenerated":true}]}}}`)
		binding := Binding{Path: "fixture.tfl", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"node-1": map[string]any{"fields": []any{map[string]any{"name": "Sales", "type": "number"}}}}}}}
		if err := VerifyNative(t.Context(), "flow", data, binding); err != nil {
			t.Fatalf("flow subset rejected extra field metadata: %v", err)
		}
	})

	tests := map[string][]byte{
		"missing authored field": []byte(`{"nodes":{"node-1":{"fields":[{"name":"Sales"}]}}}`),
		"changed authored field": []byte(`{"nodes":{"node-1":{"fields":[{"name":"Profit","type":"number"}]}}}`),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			binding := Binding{Path: "fixture.tfl", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"node-1": map[string]any{"fields": []any{map[string]any{"name": "Sales", "type": "number"}}}}}}}
			if err := VerifyNative(t.Context(), "flow", data, binding); err == nil {
				t.Fatal("flow missing or changing authored array field was accepted")
			}
		})
	}
}

func TestNativeFlowExactDocumentRejectsExtraFieldsInsideArrayObjects(t *testing.T) {
	data := []byte(`{"nodes":{"node-1":{"fields":[{"name":"Sales","type":"number","isGenerated":true}]}}}`)
	binding := Binding{Path: "fixture.tfl", Expected: map[string]any{"document": map[string]any{"nodes": map[string]any{"node-1": map[string]any{"fields": []any{map[string]any{"name": "Sales", "type": "number"}}}}}}}
	if err := VerifyNative(t.Context(), "flow", data, binding); err == nil {
		t.Fatal("exact flow document accepted an extra array-object field")
	}
}

func TestNativeFlowDocumentMismatchReportsSafeJSONPath(t *testing.T) {
	data := []byte(`{"nodes":{"output":{"name":"actual-private-value"}}}`)
	binding := Binding{Path: "fixture.tfl", Expected: map[string]any{"document": map[string]any{"nodes": map[string]any{"output": map[string]any{"name": "expected-private-value"}}}}}
	err := VerifyNative(t.Context(), "flow", data, binding)
	if err == nil {
		t.Fatal("mismatched exact flow document was accepted")
	}
	message := err.Error()
	for _, want := range []string{`$["nodes"]["output"]["name"]`, "string", "mismatch"} {
		if !strings.Contains(message, want) {
			t.Fatalf("document mismatch error omitted safe path/type detail %q: %s", want, message)
		}
	}
	for _, private := range []string{"actual-private-value", "expected-private-value"} {
		if strings.Contains(message, private) {
			t.Fatalf("document mismatch error exposed authored scalar %q", private)
		}
	}
}

func TestNativeFlowMissingKeyReportsSafeJSONPath(t *testing.T) {
	data := []byte(`{"nodes":{"output":{"name":"actual-private-value"}}}`)
	binding := Binding{Path: "fixture.tfl", Expected: map[string]any{"flow": map[string]any{"nodes": map[string]any{"output": map[string]any{"table": "unseen-private-value"}}}}}
	err := VerifyNative(t.Context(), "flow", data, binding)
	if err == nil {
		t.Fatal("flow missing an authored key was accepted")
	}
	message := err.Error()
	for _, want := range []string{`$["nodes"]["output"]["table"]`, "missing", "string"} {
		if !strings.Contains(message, want) {
			t.Fatalf("missing-key error omitted safe path/type detail %q: %s", want, message)
		}
	}
	for _, private := range []string{"actual-private-value", "unseen-private-value"} {
		if strings.Contains(message, private) {
			t.Fatalf("missing-key error exposed authored scalar %q", private)
		}
	}
}
