// Package twbschema checks Tableau workbook and datasource XML against the
// Tableau document schemas (XSD) published for each Tableau release.
//
// The schemas are stricter than Tableau's loader: real workbooks that open
// fine can fail them. Callers report issues as warnings, not as proof that
// Tableau refuses the file.
package twbschema

import (
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed schemas/*.xsd.gz
var schemaFiles embed.FS

// Issue is one place where a document departs from the schema. Path names
// the element chain from the outermost named sheet or datasource, for
// example `worksheet "Sheet 1" > table > cols`.
type Issue struct {
	Line    int
	Path    string
	Message string
}

// Result is what Validate found: the first issues it found, up to
// issueLimit, in document order, and how many more it left out.
type Result struct {
	Issues  []Issue
	Omitted int
}

// Schema is one compiled Tableau release schema. It is immutable and safe for
// concurrent use.
type Schema struct {
	version  string
	elements map[string]*elementDecl
}

// Version returns the Tableau release the schema describes, such as 2026.1.
func (s *Schema) Version() string { return s.version }

// loaded holds one compile per release, so releases compile concurrently
// and a compiled release is shared.
var loaded sync.Map // version -> func() (*Schema, error)

// Load returns the compiled schema for a Tableau release such as "2026.1".
func Load(version string) (*Schema, error) {
	if once, ok := loaded.Load(version); ok {
		return once.(func() (*Schema, error))()
	}
	versions := Versions()
	if !slices.Contains(versions, version) {
		return nil, fmt.Errorf("no Tableau schema for %q; available: %s", version, strings.Join(versions, ", "))
	}
	once, _ := loaded.LoadOrStore(version, sync.OnceValues(func() (*Schema, error) {
		src, err := readSchema(version)
		if err != nil {
			return nil, fmt.Errorf("read the Tableau %s schema: %w", version, err)
		}
		elements, err := compile(src)
		if err != nil {
			return nil, fmt.Errorf("compile the Tableau %s schema: %w", version, err)
		}
		return &Schema{version: version, elements: elements}, nil
	}))
	return once.(func() (*Schema, error))()
}

// Versions lists the Tableau releases with a schema, oldest first.
func Versions() []string {
	entries, err := schemaFiles.ReadDir("schemas")
	if err != nil {
		panic(err)
	}
	var versions []string
	for _, entry := range entries {
		if v, ok := strings.CutPrefix(entry.Name(), "twb_"); ok {
			if v, ok = strings.CutSuffix(v, ".0.xsd.gz"); ok {
				versions = append(versions, v)
			}
		}
	}
	sort.Slice(versions, func(i, j int) bool { return releaseLess(versions[i], versions[j]) })
	return versions
}

func readSchema(version string) ([]byte, error) {
	file, err := schemaFiles.Open(path.Join("schemas", "twb_"+version+".0.xsd.gz"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, unzipped); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// releaseLess orders "YYYY.R" releases numerically.
func releaseLess(a, b string) bool {
	ay, ar := splitRelease(a)
	by, br := splitRelease(b)
	if ay != by {
		return ay < by
	}
	return ar < br
}

func splitRelease(v string) (int, int) {
	year, release, _ := strings.Cut(v, ".")
	y, _ := strconv.Atoi(year)
	r, _ := strconv.Atoi(release)
	return y, r
}
