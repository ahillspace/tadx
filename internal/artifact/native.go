package artifact

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var nativePrimaryExtensions = map[string]string{"workbook": ".twb", "datasource": ".tds", "flow": ".tfl"}

// Native is a read-only native payload, not a manufactured managed artifact.
type Native struct {
	Path, Filename, Name, Fingerprint string
	Size                              int64
	CompositionStatus                 string
	ParentDataSourceURLs              []string
	// ContentURL is a datasource definition's own repository location.
	ContentURL  string
	Datasources []WorkbookDatasource
}

// ReadNative streams native structure validation and fingerprints the original
// file. Publishing adapters recheck the fingerprint before committing its bytes.
func ReadNative(ctx context.Context, path, kind string) (Native, error) {
	if err := ctx.Err(); err != nil {
		return Native{}, err
	}
	extension := strings.ToLower(filepath.Ext(path))
	isHyper := kind == "datasource" && extension == ".hyper"
	primary := nativePrimaryExtensions[kind]
	if primary == "" || (!isHyper && extension != primary && extension != primary+"x") {
		return Native{}, fmt.Errorf("unsupported native %s file extension", kind)
	}
	file, err := os.Open(path)
	if err != nil {
		return Native{}, errors.New("native file could not be opened")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return Native{}, errors.New("native file must be a nonempty regular file")
	}
	result := Native{Path: path, Filename: filepath.Base(path), Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Size: info.Size()}
	if isHyper {
		result.CompositionStatus = CompositionStatusOrdinary
	} else {
		definition, err := openNativeDefinition(ctx, file, info.Size(), kind)
		if err != nil {
			return Native{}, err
		}
		if kind == "flow" {
			err = validateNativeFlow(ctx, definition)
		} else {
			var parsed datasourceDefinition
			parsed, err = validateNativeXML(ctx, definition, kind)
			if kind == "datasource" {
				result.CompositionStatus, result.ParentDataSourceURLs, result.ContentURL = parsed.CompositionStatus, parsed.ParentDataSourceURLs, parsed.ContentURL
			}
		}
		closeErr := definition.Close()
		if err != nil {
			return Native{}, err
		}
		if closeErr != nil {
			return Native{}, errors.New("native definition could not be read")
		}
		if kind == "workbook" {
			definition, err = openNativeDefinition(ctx, file, info.Size(), kind)
			if err != nil {
				return Native{}, err
			}
			result.Datasources, err = readNativeWorkbookDatasources(ctx, definition)
			if err != nil {
				return Native{}, err
			}
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return Native{}, errors.New("native file fingerprint failed")
	}
	hash := sha256.New()
	size, err := io.CopyBuffer(hash, &contextReader{ctx: ctx, reader: file}, make([]byte, 1024*1024))
	if err != nil {
		if ctx.Err() != nil {
			return Native{}, ctx.Err()
		}
		return Native{}, errors.New("native file fingerprint failed")
	}
	after, err := file.Stat()
	if err != nil || size != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return Native{}, errors.New("native file changed during validation; retry")
	}
	result.Fingerprint = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return result, nil
}

func readNativeWorkbookDatasources(ctx context.Context, definition io.ReadCloser) ([]WorkbookDatasource, error) {
	datasources, err := walkWorkbookDefinition(ctx, &contextReader{ctx: ctx, reader: definition}, nil, nil)
	closeErr := definition.Close()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if closeErr != nil {
		return nil, errors.New("native definition could not be read")
	}
	return datasources, nil
}

// openNativeDefinition opens the one primary definition without materializing
// its bytes or imposing a ceiling on the package or definition size.
func openNativeDefinition(ctx context.Context, file *os.File, size int64, kind string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(file.Name()), nativePrimaryExtensions[kind]) {
		return io.NopCloser(io.NewSectionReader(file, 0, size)), nil
	}
	archive, err := zip.NewReader(nativeDefinitionReaderAt{ctx: ctx, source: file}, size)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("native package could not be read")
	}
	selected, err := primaryPackageEntry(archive, kind)
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return nil, errors.New("native package requires one top-level primary definition")
	}
	stream, err := selected.Open()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("native definition could not be read")
	}
	return stream, nil
}

type nativeDefinitionReaderAt struct {
	ctx    context.Context
	source io.ReaderAt
}

func (r nativeDefinitionReaderAt) ReadAt(data []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.ReadAt(data, offset)
}

func validateNativeXML(ctx context.Context, source io.Reader, kind string) (datasourceDefinition, error) {
	decoder := xml.NewDecoder(&contextReader{ctx: ctx, reader: source})
	var metadata datasourceDefinitionCollector
	depth, roots := 0, 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return datasourceDefinition{}, ctx.Err()
			}
			return datasourceDefinition{}, errors.New("native definition contains invalid XML")
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if token.Name.Local != kind {
					return datasourceDefinition{}, fmt.Errorf("native definition requires a %s root", kind)
				}
			}
			depth++
			if kind == "datasource" {
				metadata.observe(token, depth)
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(token)) != "" {
				return datasourceDefinition{}, errors.New("native definition contains text outside its root")
			}
		}
	}
	if roots != 1 || depth != 0 {
		return datasourceDefinition{}, errors.New("native definition requires one complete root")
	}
	return metadata.definition(), nil
}

func validateNativeFlow(ctx context.Context, source io.Reader) error {
	decoder := json.NewDecoder(&contextReader{ctx: ctx, reader: source})
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("native flow definition must be a JSON object")
	}
	for depth := 1; depth > 0; {
		token, err := decoder.Token()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("native flow definition must be one complete JSON object")
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("native flow definition must be one complete JSON object")
	}
	return nil
}

// primaryPackageEntry selects the one primary definition at a package's root,
// or nil when there is none. Tableau packages keep their primary definition
// there; nested definitions are ordinary package content and neither satisfy
// nor block primary selection.
func primaryPackageEntry(archive *zip.Reader, kind string) (*zip.File, error) {
	primary := nativePrimaryExtensions[kind]
	var selected *zip.File
	for _, entry := range archive.File {
		if entry.Name == "" || strings.ContainsAny(entry.Name, `/\`) {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name), primary) || (kind == "flow" && entry.Name == "flow") {
			if selected != nil {
				return nil, errors.New("native package has ambiguous top-level primary definitions")
			}
			selected = entry
		}
	}
	return selected, nil
}
