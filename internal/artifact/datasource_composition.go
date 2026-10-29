package artifact

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxDatasourceDefinitionBytes = 16 * 1024 * 1024

// datasourceDefinition is what TADX reads from a serialized datasource
// definition. ContentURL is the definition's own repository location, which
// Tableau writes into a published datasource's download; it is empty for a
// datasource that was never published.
type datasourceDefinition struct {
	CompositionStatus    string
	ParentDataSourceURLs []string
	ContentURL           string
}

// readDatasourceDefinition reads only the serialized datasource definition.
// It never rewrites the native package and reports unknown composition for an
// unreadable package so a pull can still preserve the authoritative Tableau
// download.
func readDatasourceDefinition(filename string, source io.ReaderAt, size int64) datasourceDefinition {
	unknown := datasourceDefinition{CompositionStatus: CompositionStatusUnknown}
	definition := io.Reader(io.NewSectionReader(source, 0, size))
	var packaged *io.LimitedReader
	if strings.EqualFold(filepath.Ext(filename), ".tdsx") {
		reader, err := zip.NewReader(source, size)
		if err != nil {
			return unknown
		}
		selected, err := primaryPackageEntry(reader, "datasource")
		if err != nil || selected == nil || selected.UncompressedSize64 > maxDatasourceDefinitionBytes {
			return unknown
		}
		stream, err := selected.Open()
		if err != nil {
			return unknown
		}
		defer stream.Close()
		packaged = &io.LimitedReader{R: stream, N: maxDatasourceDefinitionBytes + 1}
		definition = packaged
	}
	decoder := xml.NewDecoder(definition)
	parents := map[string]struct{}{}
	var contentURL string
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil || (packaged != nil && packaged.N <= 0) {
			return unknown
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
		for _, attribute := range start.Attr {
			if attribute.Name.Local == "datasource-url" {
				value := strings.TrimSpace(attribute.Value)
				if value != "" {
					parents[value] = struct{}{}
				}
			}
			if depth == 2 && start.Name.Local == "repository-location" && attribute.Name.Local == "id" {
				contentURL = attribute.Value
			}
		}
	}
	values := make([]string, 0, len(parents))
	for value := range parents {
		values = append(values, value)
	}
	sort.Strings(values)
	if len(values) == 0 {
		return datasourceDefinition{CompositionStatus: CompositionStatusOrdinary, ContentURL: contentURL}
	}
	return datasourceDefinition{CompositionStatus: CompositionStatusComposed, ParentDataSourceURLs: values, ContentURL: contentURL}
}

// readDatasourceContentURL reports the repository location recorded in one
// datasource file, or empty when the file records none.
func readDatasourceContentURL(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	return readDatasourceDefinition(path, file, info.Size()).ContentURL, nil
}
