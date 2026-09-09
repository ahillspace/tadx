package artifact

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
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
