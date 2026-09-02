package artifact

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

const maxDatasourceDefinitionBytes = 16 * 1024 * 1024

// classifyDatasourcePackage reads only the serialized datasource definition.
// It never rewrites the native package and returns unknown for an unreadable
// package so a pull can still preserve the authoritative Tableau download.
func classifyDatasourcePackage(filename string, content []byte) (string, []string) {
	definition := content
	if strings.EqualFold(filepath.Ext(filename), ".tdsx") {
		reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err != nil {
			return CompositionStatusUnknown, nil
		}
		var selected *zip.File
		for _, file := range reader.File {
			if strings.EqualFold(filepath.Ext(file.Name), ".tds") {
				if selected != nil {
					return CompositionStatusUnknown, nil
				}
				selected = file
			}
		}
		if selected == nil || selected.UncompressedSize64 > maxDatasourceDefinitionBytes {
			return CompositionStatusUnknown, nil
		}
		stream, err := selected.Open()
		if err != nil {
			return CompositionStatusUnknown, nil
		}
		definition, err = io.ReadAll(io.LimitReader(stream, maxDatasourceDefinitionBytes+1))
		closeErr := stream.Close()
		if err != nil || closeErr != nil || len(definition) > maxDatasourceDefinitionBytes {
			return CompositionStatusUnknown, nil
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(definition))
	parents := map[string]struct{}{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return CompositionStatusUnknown, nil
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Local == "datasource-url" {
				value := strings.TrimSpace(attribute.Value)
				if value != "" {
					parents[value] = struct{}{}
				}
			}
		}
	}
	values := make([]string, 0, len(parents))
	for value := range parents {
		values = append(values, value)
	}
	sort.Strings(values)
	if len(values) == 0 {
		return CompositionStatusOrdinary, nil
	}
	return CompositionStatusComposed, values
}
