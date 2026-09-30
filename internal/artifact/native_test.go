package artifact

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadNativeValidatesWithoutWritingOrRewriting(t *testing.T) {
	for _, test := range []struct {
		name, kind, body string
		valid            bool
	}{
		{"book.twb", "workbook", "<workbook/>", true}, {"source.tds", "datasource", "<datasource><relation datasource-url=\"parent\"/></datasource>", true}, {"prep.tfl", "flow", `{"nodes":{}}`, true},
		{"wrong.twb", "workbook", "<datasource/>", false}, {"broken.tds", "datasource", "<datasource>", false}, {"bad.tfl", "flow", "[]", false}, {"bad.exe", "workbook", "<workbook/>", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), test.name)
			os.WriteFile(path, []byte(test.body), 0600)
			result, err := ReadNative(context.Background(), path, test.kind)
			if (err == nil) != test.valid {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			after, _ := os.ReadFile(path)
			if string(after) != test.body {
				t.Fatal("native payload changed")
			}
			if test.valid && (result.Fingerprint == "" || result.Size != int64(len(test.body))) {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}

func TestReadNativePackageRequiresUnambiguousBoundedDefinition(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		var data bytes.Buffer
		archive := zip.NewWriter(&data)
		for i := 0; i < count; i++ {
			name := []string{"first.twb", "second.twb"}[i]
			entry, _ := archive.Create(name)
			entry.Write([]byte("<workbook/>"))
		}
		archive.Close()
		path := filepath.Join(t.TempDir(), "book.twbx")
		os.WriteFile(path, data.Bytes(), 0600)
		_, err := ReadNative(context.Background(), path, "workbook")
		if (err == nil) != (count == 1) {
			t.Fatalf("count%d err=%v", count, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadNative(ctx, "missing.twb", "workbook"); err != context.Canceled {
		t.Fatalf("cancel=%v", err)
	}
}

func TestReadNativePackageUsesOnlyTopLevelDefinition(t *testing.T) {
	for _, test := range []struct {
		name, kind, file string
		entries          [][2]string
		valid            bool
		composition      string
	}{
		{"nested workbook only", "workbook", "book.twbx", [][2]string{{"Data/deep/inner.twb", "<workbook/>"}}, false, ""},
		{"backslash nested workbook only", "workbook", "book.twbx", [][2]string{{`Data\inner.twb`, "<workbook/>"}}, false, ""},
		{"top workbook with nested workbook", "workbook", "book.twbx", [][2]string{{"top.twb", "<workbook/>"}, {"Data/extra/embedded.twb", "<workbook>"}}, true, ""},
		{"top datasource with nested datasource", "datasource", "source.tdsx", [][2]string{{"real.tds", "<datasource/>"}, {"Data/sub/other.tds", `<datasource><relation datasource-url="parent"/></datasource>`}}, true, CompositionStatusOrdinary},
		{"nested datasource only", "datasource", "source.tdsx", [][2]string{{"Data/sub/other.tds", "<datasource/>"}}, false, ""},
		{"top flow with nested flow", "flow", "prep.tflx", [][2]string{{"flow", `{"nodes":{}}`}, {"Data/flow", "[]"}}, true, ""},
		{"nested flow only", "flow", "prep.tflx", [][2]string{{"Data/flow", `{"nodes":{}}`}}, false, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			var data bytes.Buffer
			archive := zip.NewWriter(&data)
			for _, item := range test.entries {
				entry, err := archive.Create(item[0])
				if err != nil {
					t.Fatal(err)
				}
				entry.Write([]byte(item[1]))
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), test.file)
			if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := ReadNative(context.Background(), path, test.kind)
			if (err == nil) != test.valid {
				t.Fatalf("result=%#v err=%v", result, err)
			}
			if err != nil && !strings.Contains(err.Error(), "top-level") {
				t.Fatalf("error does not name the top-level rule: %v", err)
			}
			if result.CompositionStatus != test.composition {
				t.Fatalf("composition = %q", result.CompositionStatus)
			}
		})
	}
}
