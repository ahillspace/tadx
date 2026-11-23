package twbschema_test

import (
	"archive/zip"
	"bufio"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/twbschema"
)

// TestLocalCorpus validates workbooks and datasources kept outside the
// repository. TWBSCHEMA_CORPUS names a file that lists one .twb, .twbx, .tds,
// or .tdsx path per line. Every document must select a schema, read as XML,
// and have a root the schema declares; issue counts are logged so a change
// to the validator can be compared run to run.
func TestLocalCorpus(t *testing.T) {
	list := os.Getenv("TWBSCHEMA_CORPUS")
	if list == "" {
		t.Skip("TWBSCHEMA_CORPUS is not set")
	}
	f, err := os.Open(list)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		file := strings.TrimSpace(scanner.Text())
		if file == "" {
			continue
		}
		// Subtests are named by list line so no local path reaches the output.
		t.Run("line_"+strconv.Itoa(n), func(t *testing.T) {
			doc := rootDocument(t, file)
			schema, note, err := twbschema.Select(doc)
			if err != nil {
				t.Fatal(err)
			}
			result, err := schema.Validate(context.Background(), doc)
			if err != nil {
				t.Fatal(err)
			}
			for _, issue := range result.Issues {
				if strings.Contains(issue.Message, "is not a Tableau workbook or datasource root") {
					t.Fatalf("line %d: %s", issue.Line, issue.Message)
				}
			}
			t.Logf("schema %s, %d issues, %d omitted %s", schema.Version(), len(result.Issues), result.Omitted, note)
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func rootDocument(t *testing.T, file string) []byte {
	t.Helper()
	switch filepath.Ext(file) {
	case ".twb", ".tds":
		doc, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	r, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, entry := range r.File {
		if ext := path.Ext(entry.Name); !strings.Contains(entry.Name, "/") && (ext == ".twb" || ext == ".tds") {
			rc, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			doc, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			return doc
		}
	}
	t.Fatal("the package has no root document")
	return nil
}
