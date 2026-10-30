package artifact

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// datasourceDefinition is what TADX reads from a serialized datasource
// definition. ContentURL is the definition's own repository location, which
// Tableau writes into a published datasource's download; it is empty for a
// datasource that was never published.
type datasourceDefinition struct {
	CompositionStatus    string
	ParentDataSourceURLs []string
	ContentURL           string
}

// datasourceDefinitionCollector keeps only the metadata needed by push and pull
// while their definition readers enforce their own validation contracts.
type datasourceDefinitionCollector struct {
	parents    map[string]struct{}
	contentURL string
}

func (c *datasourceDefinitionCollector) observe(start xml.StartElement, depth int) {
	for _, attribute := range start.Attr {
		if attribute.Name.Local == "datasource-url" {
			if value := strings.TrimSpace(attribute.Value); value != "" {
				if c.parents == nil {
					c.parents = make(map[string]struct{})
				}
				c.parents[value] = struct{}{}
			}
		}
		if depth == 2 && start.Name.Local == "repository-location" && attribute.Name.Local == "id" {
			c.contentURL = attribute.Value
		}
	}
}

func (c *datasourceDefinitionCollector) definition() datasourceDefinition {
	if len(c.parents) == 0 {
		return datasourceDefinition{CompositionStatus: CompositionStatusOrdinary, ContentURL: c.contentURL}
	}
	values := make([]string, 0, len(c.parents))
	for value := range c.parents {
		values = append(values, value)
	}
	sort.Strings(values)
	return datasourceDefinition{CompositionStatus: CompositionStatusComposed, ParentDataSourceURLs: values, ContentURL: c.contentURL}
}

// readDatasourceDefinition reads only the serialized datasource definition.
// It never rewrites the native package and reports unknown composition for an
// unreadable package so a pull can still preserve the authoritative Tableau
// download.
func readDatasourceDefinition(filename string, source io.ReaderAt, size int64) datasourceDefinition {
	return readDatasourceDefinitionContext(context.Background(), filename, source, size)
}

func readDatasourceDefinitionContext(ctx context.Context, filename string, source io.ReaderAt, size int64) datasourceDefinition {
	unknown := datasourceDefinition{CompositionStatus: CompositionStatusUnknown}
	source = nativeDefinitionReaderAt{ctx: ctx, source: source}
	definition := io.Reader(io.NewSectionReader(source, 0, size))
	if strings.EqualFold(filepath.Ext(filename), ".tdsx") {
		reader, err := zip.NewReader(source, size)
		if err != nil {
			return unknown
		}
		selected, err := primaryPackageEntry(reader, "datasource")
		if err != nil || selected == nil {
			return unknown
		}
		stream, err := selected.Open()
		if err != nil {
			return unknown
		}
		defer stream.Close()
		definition = stream
	}
	return readDatasourceDefinitionStream(&contextReader{ctx: ctx, reader: definition})
}

func readDatasourceDefinitionStream(definition io.Reader) datasourceDefinition {
	decoder := xml.NewDecoder(definition)
	var metadata datasourceDefinitionCollector
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return datasourceDefinition{CompositionStatus: CompositionStatusUnknown}
		}
		if _, ok := token.(xml.EndElement); ok {
			depth--
			continue
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		depth++
		metadata.observe(start, depth)
	}
	return metadata.definition()
}

// readDatasourceFile reads the datasource definition at path, using filename
// to distinguish the original TDS or TDSX name from a temporary staging name.
func readDatasourceFile(ctx context.Context, filename, path string) (datasourceDefinition, error) {
	if err := ctx.Err(); err != nil {
		return datasourceDefinition{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return datasourceDefinition{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return datasourceDefinition{}, err
	}
	parsed := readDatasourceDefinitionContext(ctx, filename, file, info.Size())
	if err := ctx.Err(); err != nil {
		return datasourceDefinition{}, err
	}
	return parsed, nil
}

// readDatasourceContentURL reports the repository location recorded in one
// datasource file, or empty when the file records none.
func readDatasourceContentURL(ctx context.Context, path string) (string, error) {
	parsed, err := readDatasourceFile(ctx, path, path)
	return parsed.ContentURL, err
}
