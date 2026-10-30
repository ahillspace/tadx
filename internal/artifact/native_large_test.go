package artifact

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLargeNativeDefinition(t *testing.T, output io.Writer, kind string) {
	t.Helper()
	var prefix, block, suffix string
	if kind == "flow" {
		prefix, block, suffix = `{"nodes":[`, strings.Repeat(`{},`, 4096), `{}],"largeNumber":1e10000}`
	} else {
		prefix, block, suffix = "<"+kind+">", strings.Repeat("<item/>", 4096), "</"+kind+">"
		if kind == "datasource" {
			prefix += `<repository-location id="LargeSource"/><relation datasource-url="parent"/>`
		}
	}
	if _, err := io.WriteString(output, prefix); err != nil {
		t.Fatal(err)
	}
	for range 17*1024*1024/len(block) + 1 {
		if _, err := io.WriteString(output, block); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := io.WriteString(output, suffix); err != nil {
		t.Fatal(err)
	}
}

func TestReadNativeStreamsLargePlainAndPackagedDefinitions(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, packaged := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/package=%t", kind, packaged), func(t *testing.T) {
				extension := nativePrimaryExtensions[kind]
				filename := "large" + extension
				if packaged {
					filename += "x"
				}
				path := filepath.Join(t.TempDir(), filename)
				file, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if packaged {
					archive := zip.NewWriter(file)
					definition, err := archive.Create("definition" + extension)
					if err != nil {
						t.Fatal(err)
					}
					writeLargeNativeDefinition(t, definition, kind)
					if err := archive.Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					writeLargeNativeDefinition(t, file, kind)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				got, err := ReadNative(t.Context(), path, kind)
				if err != nil {
					t.Fatal(err)
				}
				if got.Fingerprint == "" {
					t.Fatal("missing fingerprint")
				}
				if kind == "datasource" {
					if got.ContentURL != "LargeSource" || got.CompositionStatus != CompositionStatusComposed || len(got.ParentDataSourceURLs) != 1 || got.ParentDataSourceURLs[0] != "parent" {
						t.Fatalf("large datasource metadata = %#v", got)
					}
					if contentURL, err := readDatasourceContentURL(t.Context(), path); err != nil || contentURL != "LargeSource" {
						t.Fatalf("managed datasource content URL = %q, error=%v", contentURL, err)
					}
				}
			})
		}
	}
}

func TestReadNativeFingerprintsLargeHyperWithoutPayloadBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.hyper")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	const size int64 = 1<<30 + 1
	if err := file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	native, err := ReadNative(t.Context(), path, "datasource")
	if err != nil {
		t.Fatal(err)
	}
	if native.Size != size {
		t.Fatalf("size=%d, want %d", native.Size, size)
	}
	file, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if n, err := io.Copy(hash, file); err != nil || n != size {
		t.Fatalf("independent fingerprint read=%d, error=%v", n, err)
	}
	if want := "sha256:" + hex.EncodeToString(hash.Sum(nil)); native.Fingerprint != want {
		t.Fatalf("fingerprint=%q, want %q", native.Fingerprint, want)
	}
}

func TestReadNativePackageAcceptsManyPayloadEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.twbx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	definition, err := archive.Create("book.twb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(definition, "<workbook/>"); err != nil {
		t.Fatal(err)
	}
	for i := range 10001 {
		if _, err := archive.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("Data/entry-%d", i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadNative(t.Context(), path, "workbook"); err != nil {
		t.Fatal(err)
	}
}

func TestReadNativeFlowRequiresExactlyOneCompleteObject(t *testing.T) {
	for _, body := range []string{`{} {}`, `{} null`, `{"nodes":[}`, `{"nodes":{}`, `null`, `[{}]`} {
		t.Run(body, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid.tfl")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadNative(t.Context(), path, "flow"); err == nil {
				t.Fatal("accepted an incomplete or non-object flow definition")
			}
		})
	}
}

type nativeCancelAfterRead struct {
	io.Reader
	cancel context.CancelFunc
}

func (r nativeCancelAfterRead) Read(data []byte) (int, error) {
	n, err := r.Reader.Read(data)
	r.cancel()
	return n, err
}

func TestNativeDefinitionValidationObservesCancellationDuringRead(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			body := "<" + kind + "/>"
			if kind == "flow" {
				body = `{"nodes":{}}`
			}
			source := nativeCancelAfterRead{Reader: strings.NewReader(body), cancel: cancel}
			var err error
			if kind == "flow" {
				err = validateNativeFlow(ctx, source)
			} else {
				_, err = validateNativeXML(ctx, source, kind)
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v, want cancellation during definition read", err)
			}
		})
	}
}

func TestNativeWorkbookDatasourceScanPreservesCancellation(t *testing.T) {
	const body = `<workbook><datasources><datasource caption="Orders"><connection class="sqlproxy" dbname="Orders"/></datasource></datasources></workbook>`
	if _, err := validateNativeXML(t.Context(), strings.NewReader(body), "workbook"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	definition := io.NopCloser(nativeCancelAfterRead{Reader: strings.NewReader(body), cancel: cancel})
	if _, err := readNativeWorkbookDatasources(ctx, definition); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want cancellation while reading native workbook datasources", err)
	}
}
