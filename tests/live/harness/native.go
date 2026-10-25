package harness

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

type NativeEvidence struct {
	Document                               string
	XMLRoot                                string
	Members, Columns, Worksheets, Extracts []string
	Payloads                               map[string][]byte
}

// ValidateNative checks native syntax, archive safety, and authored semantic markers.
// It does not use any production artifact parser.
func ValidateNative(kind string, n Native) (NativeEvidence, error) {
	evidence := NativeEvidence{Payloads: map[string][]byte{}}
	if len(n.Bytes) == 0 {
		return evidence, errors.New("native artifact is empty")
	}
	archiveInput := bytes.HasPrefix(n.Bytes, []byte("PK\x03\x04"))
	if archiveInput {
		archive, err := zip.NewReader(bytes.NewReader(n.Bytes), int64(len(n.Bytes)))
		if err != nil {
			return evidence, fmt.Errorf("native ZIP: %w", err)
		}
		var total uint64
		for _, member := range archive.File {
			name := strings.ReplaceAll(member.Name, "\\", "/")
			if member.FileInfo().IsDir() {
				name = strings.TrimSuffix(name, "/")
			}
			clean := path.Clean(name)
			if name != clean || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, ":") {
				return evidence, errors.New("native ZIP contains unsafe member path")
			}
			if member.Mode()&os.ModeSymlink != 0 {
				return evidence, errors.New("native ZIP contains symlink member")
			}
			if member.FileInfo().IsDir() {
				continue
			}
			if _, exists := evidence.Payloads[name]; exists {
				return evidence, errors.New("native ZIP contains duplicate members")
			}
			total += member.UncompressedSize64
			if total > 1<<30 {
				return evidence, errors.New("native ZIP expands beyond 1 GiB")
			}
			reader, err := member.Open()
			if err != nil {
				return evidence, err
			}
			data, readErr := io.ReadAll(io.LimitReader(reader, (512<<20)+1))
			closeErr := reader.Close()
			if readErr != nil {
				return evidence, readErr
			}
			if closeErr != nil {
				return evidence, closeErr
			}
			if len(data) > 512<<20 {
				return evidence, errors.New("native ZIP member exceeds 512 MiB")
			}
			evidence.Payloads[name] = data
			evidence.Members = append(evidence.Members, name)
			if strings.EqualFold(path.Ext(name), ".hyper") {
				evidence.Extracts = append(evidence.Extracts, name)
			}
		}
	} else {
		evidence.Payloads[n.Filename] = n.Bytes
		evidence.Members = []string{n.Filename}
		if strings.EqualFold(filepath.Ext(n.Filename), ".hyper") {
			evidence.Extracts = []string{n.Filename}
		}
	}
	slices.Sort(evidence.Members)
	slices.Sort(evidence.Extracts)
	ext := map[string]string{"workbook": ".twb", "datasource": ".tds", "flow": ".tfl"}[kind]
	if ext == "" {
		return evidence, errors.New("unknown native artifact kind")
	}
	for _, name := range evidence.Members {
		flowMember := kind == "flow" && name == "flow"
		if strings.EqualFold(path.Ext(name), ext) || flowMember {
			if evidence.Document != "" {
				return evidence, errors.New("native package contains multiple primary documents")
			}
			evidence.Document = name
		}
	}
	if evidence.Document == "" {
		if kind == "datasource" && len(evidence.Extracts) == 1 {
			return evidence, nil
		}
		if kind == "flow" && !archiveInput {
			switch strings.ToLower(filepath.Ext(n.Filename)) {
			case ".tfl", ".tflx":
				evidence.Document = n.Filename
			}
		}
	}
	if evidence.Document == "" {
		if kind == "flow" {
			return evidence, errors.New("native flow package omits flow or .tfl document")
		}
		return evidence, fmt.Errorf("native package omits %s document", ext)
	}
	data := evidence.Payloads[evidence.Document]
	if kind == "flow" {
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			return evidence, errors.New("native flow is not a JSON object")
		}
		if len(document) == 0 {
			return evidence, errors.New("native flow document is empty")
		}
		return evidence, nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	columnMetadataDepth := 0
	roots := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return evidence, fmt.Errorf("native XML: %w", err)
		}
		switch item := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				evidence.XMLRoot = item.Name.Local
			}
			depth++
			if item.Name.Local == "metadata-record" && hasXMLAttribute(item.Attr, "class", "column") {
				columnMetadataDepth = depth
			}
			switch item.Name.Local {
			case "column":
				for _, attr := range item.Attr {
					if attr.Name.Local == "name" {
						evidence.Columns = append(evidence.Columns, attr.Value)
					}
				}
			case "worksheet":
				for _, attr := range item.Attr {
					if attr.Name.Local == "name" {
						evidence.Worksheets = append(evidence.Worksheets, attr.Value)
					}
				}
			case "local-name":
				if columnMetadataDepth > 0 {
					var name string
					if err := decoder.DecodeElement(&name, &item); err != nil {
						return evidence, fmt.Errorf("native XML column metadata: %w", err)
					}
					depth--
					if name != "" {
						evidence.Columns = append(evidence.Columns, name)
					}
				}
			}
		case xml.EndElement:
			if depth == columnMetadataDepth {
				columnMetadataDepth = 0
			}
			depth--
		}
	}
	if roots != 1 || evidence.XMLRoot != kind {
		return evidence, fmt.Errorf("native XML root must be %s", kind)
	}
	slices.Sort(evidence.Columns)
	evidence.Columns = slices.Compact(evidence.Columns)
	slices.Sort(evidence.Worksheets)
	return evidence, nil
}

func hasXMLAttribute(attributes []xml.Attr, name, value string) bool {
	for _, attribute := range attributes {
		if attribute.Name.Local == name && attribute.Value == value {
			return true
		}
	}
	return false
}

// CompareNative compares logical XML or JSON content and every embedded payload.
// ZIP order, compression, and XML indentation do not affect the comparison.
func CompareNative(kind string, a, b []byte) error {
	ext := map[string]string{"workbook": ".twb", "datasource": ".tds", "flow": ".tfl"}[kind]
	left, err := ValidateNative(kind, Native{Bytes: a, Filename: "content" + ext})
	if err != nil {
		return err
	}
	right, err := ValidateNative(kind, Native{Bytes: b, Filename: "content" + ext})
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(left.Columns, right.Columns) || !reflect.DeepEqual(left.Worksheets, right.Worksheets) {
		return errors.New("native artifacts differ in columns or worksheets")
	}
	if len(left.Extracts) != len(right.Extracts) {
		return errors.New("native artifacts differ in embedded extract count")
	}
	if (left.Document == "") != (right.Document == "") {
		return errors.New("native artifacts differ in primary document presence")
	}
	if len(left.Payloads) != len(right.Payloads) {
		return errors.New("native artifacts differ in embedded payload inventory")
	}
	if left.Document != "" && right.Document != "" {
		x, err := canonicalDocument(kind, left.Payloads[left.Document])
		if err != nil {
			return err
		}
		y, err := canonicalDocument(kind, right.Payloads[right.Document])
		if err != nil {
			return err
		}
		if !bytes.Equal(x, y) {
			return errors.New("native artifact documents differ")
		}
	}
	for _, name := range left.Members {
		if name == left.Document {
			continue
		}
		payload, exists := right.Payloads[name]
		if !exists || name == right.Document {
			return errors.New("native artifacts differ in embedded payload inventory")
		}
		if !bytes.Equal(left.Payloads[name], payload) {
			if strings.EqualFold(path.Ext(name), ".hyper") {
				return errors.New("native artifact extract payloads differ; use authored Hyper row oracle for server-rewritten extracts")
			}
			return errors.New("native artifact embedded payloads differ")
		}
	}
	return nil
}

func canonicalDocument(kind string, data []byte) ([]byte, error) {
	if kind == "flow" {
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return json.Marshal(value)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var buffer bytes.Buffer
	encoder := xml.NewEncoder(&buffer)
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if text, ok := token.(xml.CharData); ok && len(bytes.TrimSpace(text)) == 0 {
			continue
		}
		if _, ok := token.(xml.ProcInst); ok {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		if start, ok := token.(xml.StartElement); ok {
			slices.SortFunc(start.Attr, func(a, b xml.Attr) int {
				return strings.Compare(a.Name.Space+":"+a.Name.Local, b.Name.Space+":"+b.Name.Local)
			})
			token = start
		}
		if err := encoder.EncodeToken(token); err != nil {
			return nil, err
		}
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// VerifyNative checks explicit authored markers and exact Hyper row multisets.
func VerifyNative(ctx context.Context, kind string, data []byte, binding Binding) error {
	filename := binding.Path
	if filename == "" {
		filename = "content" + map[string]string{"datasource": ".tds", "workbook": ".twb", "flow": ".tfl"}[kind]
	}
	evidence, err := ValidateNative(kind, Native{Bytes: data, Filename: filename})
	if err != nil {
		return err
	}
	checks := 0
	for key, actual := range map[string][]string{"columns": evidence.Columns, "worksheets": evidence.Worksheets} {
		if expected, ok := binding.Expected[key]; ok {
			values, err := stringList(expected)
			if err != nil {
				return fmt.Errorf("native %s manifest: %w", key, err)
			}
			for _, value := range values {
				checks++
				if !slices.Contains(actual, value) {
					return fmt.Errorf("native artifact omits expected %s marker %q", key, value)
				}
			}
		}
	}
	if count, ok := binding.Expected["extract_count"]; ok {
		checks++
		encoded, _ := json.Marshal(count)
		if string(encoded) != fmt.Sprint(len(evidence.Extracts)) {
			return errors.New("native artifact extract count differs from authored manifest")
		}
	}
	if rows, ok := binding.Expected["rows"]; ok {
		checks++
		if len(evidence.Extracts) != 1 {
			return errors.New("exact Hyper row oracle requires one extract")
		}
		directory, err := os.MkdirTemp("", "tadx-native-oracle-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(directory)
		hyperPath := filepath.Join(directory, "actual.hyper")
		if err := os.WriteFile(hyperPath, evidence.Payloads[evidence.Extracts[0]], 0600); err != nil {
			return err
		}
		actual, err := hyperRows(ctx, hyperPath, binding.Params["hyper_table"])
		if err != nil {
			return err
		}
		expectedJSON, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		var expected []any
		if err := json.Unmarshal(expectedJSON, &expected); err != nil {
			return errors.New("expected Hyper rows must be a JSON array")
		}
		if !sameRows(actual, expected) {
			return errors.New("native Hyper rows differ from authored row multiset")
		}
	}
	if kind == "flow" {
		checks = 0
		var actual map[string]any
		if err := json.Unmarshal(evidence.Payloads[evidence.Document], &actual); err != nil {
			return err
		}
		if expected, ok := binding.Expected["document"]; ok {
			checks++
			encoded, err := json.Marshal(expected)
			if err != nil {
				return err
			}
			var normalized any
			if json.Unmarshal(encoded, &normalized) != nil {
				return errors.New("native flow document expectation is invalid")
			}
			if mismatch := nativeJSONMismatch(normalized, actual, "$", false); mismatch != "" {
				return fmt.Errorf("native flow document mismatch: %s", mismatch)
			}
		}
		if expected, ok := binding.Expected["flow"]; ok {
			checks++
			encoded, err := json.Marshal(expected)
			if err != nil {
				return err
			}
			var normalized any
			if json.Unmarshal(encoded, &normalized) != nil {
				return errors.New("native flow expectation is invalid")
			}
			if mismatch := nativeJSONMismatch(normalized, actual, "$", true); mismatch != "" {
				return fmt.Errorf("native flow expectation mismatch: %s", mismatch)
			}
		}
	}
	if checks == 0 {
		return Blocked("native artifact requires an authored semantic manifest")
	}
	return nil
}

func nativeJSONMismatch(expected, actual any, path string, allowExtraObjectMembers bool) string {
	if fields, ok := expected.(map[string]any); ok {
		got, ok := actual.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s has a type mismatch (expected object, found %s)", path, nativeJSONType(actual))
		}
		if allowExtraObjectMembers && len(fields) == 0 {
			return fmt.Sprintf("%s has an empty authored object", path)
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			value := fields[key]
			candidate, exists := got[key]
			childPath := nativeJSONPathMember(path, key)
			if !exists {
				return fmt.Sprintf("%s is missing (expected %s)", childPath, nativeJSONType(value))
			}
			if mismatch := nativeJSONMismatch(value, candidate, childPath, allowExtraObjectMembers); mismatch != "" {
				return mismatch
			}
		}
		if !allowExtraObjectMembers {
			actualKeys := make([]string, 0, len(got))
			for key := range got {
				actualKeys = append(actualKeys, key)
			}
			slices.Sort(actualKeys)
			for _, key := range actualKeys {
				if _, exists := fields[key]; !exists {
					return fmt.Sprintf("%s is unexpected", nativeJSONPathMember(path, key))
				}
			}
		}
		return ""
	}
	if expectedArray, ok := expected.([]any); ok {
		actualArray, ok := actual.([]any)
		if !ok {
			return fmt.Sprintf("%s has a type mismatch (expected array, found %s)", path, nativeJSONType(actual))
		}
		if len(expectedArray) != len(actualArray) {
			return fmt.Sprintf("%s has different array lengths (expected %d, found %d)", path, len(expectedArray), len(actualArray))
		}
		for index, value := range expectedArray {
			if mismatch := nativeJSONMismatch(value, actualArray[index], fmt.Sprintf("%s[%d]", path, index), allowExtraObjectMembers); mismatch != "" {
				return mismatch
			}
		}
		return ""
	}
	if !reflect.DeepEqual(expected, actual) {
		return fmt.Sprintf("%s has a scalar mismatch (expected %s, found %s)", path, nativeJSONType(expected), nativeJSONType(actual))
	}
	return ""
}

func nativeJSONPathMember(path, key string) string {
	encoded, _ := json.Marshal(key)
	return path + "[" + string(encoded) + "]"
}

func nativeJSONType(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func CheckNative(ctx context.Context, kind string, data []byte, binding Binding) error {
	return VerifyNative(ctx, kind, data, binding)
}
func stringList(value any) ([]string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var values []string
	err = json.Unmarshal(encoded, &values)
	return values, err
}
func sameRows(a, b []any) bool {
	encode := func(rows []any) []string {
		result := make([]string, 0, len(rows))
		for _, row := range rows {
			value, _ := json.Marshal(row)
			result = append(result, string(value))
		}
		slices.Sort(result)
		return result
	}
	return slices.Equal(encode(a), encode(b))
}
func hyperRows(ctx context.Context, filename, table string) ([]any, error) {
	if table == "" {
		table = `"Extract"."Extract"`
	}
	// The SDK parses native Hyper data. SQL identifiers remain separate from shell text.
	script := `import sys,json,datetime,decimal
from tableauhyperapi import HyperProcess,Connection,Telemetry
with HyperProcess(Telemetry.DO_NOT_SEND_USAGE_DATA_TO_TABLEAU,parameters={"log_config":""}) as hyper:
 with Connection(hyper.endpoint,sys.argv[1]) as connection:
  rows=connection.execute_list_query('SELECT * FROM '+sys.argv[2])
  def encode(v):
   if isinstance(v,(datetime.date,datetime.datetime,decimal.Decimal)): return str(v)
   return str(v)
  print(json.dumps(rows,default=encode))`
	python := os.Getenv("TADX_LIVECLI_PYTHON")
	if python == "" {
		python = "python"
	}
	command := exec.CommandContext(ctx, python, "-c", script, filename, table)
	command.Dir = filepath.Dir(filename)
	output, err := command.Output()
	if err != nil {
		return nil, errors.New("independent Hyper oracle failed; install tableauhyperapi in the Python environment")
	}
	var rows []any
	if json.Unmarshal(output, &rows) != nil {
		return nil, errors.New("Hyper oracle returned invalid rows")
	}
	return rows, nil
}
