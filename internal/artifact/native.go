package artifact

import (
	"archive/zip"
	"bytes"
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

const maxNativeBytes int64 = 1 << 30

// Native is a read-only native payload, not a manufactured managed artifact.
type Native struct {
	Path, Filename, Name, Fingerprint string
	Size                              int64
	CompositionStatus                 string
	ParentDataSourceURLs              []string
}

// ReadNative validates bounded native structure and fingerprints the original file.
// Publishing adapters recheck the fingerprint before committing its bytes.
func ReadNative(ctx context.Context, path, kind string) (Native, error) {
	if err := ctx.Err(); err != nil {
		return Native{}, err
	}
	extension := strings.ToLower(filepath.Ext(path))
	isHyper := kind == "datasource" && extension == ".hyper"
	primary := map[string]string{"workbook": ".twb", "datasource": ".tds", "flow": ".tfl"}[kind]
	if primary == "" || (!isHyper && extension != primary && extension != primary+"x") {
		return Native{}, fmt.Errorf("unsupported native %s file extension", kind)
	}
	file, err := os.Open(path)
	if err != nil {
		return Native{}, errors.New("native file could not be opened")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxNativeBytes {
		return Native{}, errors.New("native file must be a nonempty regular file no larger than 1 GiB")
	}
	var definition []byte
	if !isHyper {
		if extension == primary {
			if info.Size() > maxDatasourceDefinitionBytes {
				return Native{}, errors.New("native definition exceeds 16 MiB")
			}
			definition, err = io.ReadAll(io.LimitReader(file, maxDatasourceDefinitionBytes+1))
		} else {
			var archive *zip.Reader
			archive, err = zip.NewReader(file, info.Size())
			if err == nil {
				if len(archive.File) > 10000 {
					return Native{}, errors.New("native package exceeds 10000 entries")
				}
				var selected *zip.File
				for _, entry := range archive.File {
					if strings.EqualFold(filepath.Ext(entry.Name), primary) || (kind == "flow" && entry.Name == "flow") {
						if selected != nil {
							return Native{}, errors.New("native package has ambiguous primary definitions")
						}
						selected = entry
					}
				}
				if selected == nil || selected.UncompressedSize64 > maxDatasourceDefinitionBytes {
					return Native{}, errors.New("native package requires one primary definition no larger than 16 MiB")
				}
				var stream io.ReadCloser
				stream, err = selected.Open()
				if err == nil {
					definition, err = io.ReadAll(io.LimitReader(stream, maxDatasourceDefinitionBytes+1))
					closeErr := stream.Close()
					if err == nil {
						err = closeErr
					}
				}
			}
		}
		if err != nil || len(definition) > maxDatasourceDefinitionBytes {
			return Native{}, errors.New("native definition could not be read within its bound")
		}
		if kind == "flow" {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(definition, &object); err != nil || object == nil {
				return Native{}, errors.New("native flow definition must be a JSON object")
			}
		} else {
			decoder := xml.NewDecoder(bytes.NewReader(definition))
			depth, roots := 0, 0
			for {
				token, decodeErr := decoder.Token()
				if decodeErr == io.EOF {
					break
				}
				if decodeErr != nil {
					return Native{}, errors.New("native definition contains invalid XML")
				}
				switch token := token.(type) {
				case xml.StartElement:
					if depth == 0 {
						roots++
						if token.Name.Local != kind {
							return Native{}, fmt.Errorf("native definition requires a %s root", kind)
						}
					}
					depth++
				case xml.EndElement:
					depth--
				case xml.CharData:
					if depth == 0 && strings.TrimSpace(string(token)) != "" {
						return Native{}, errors.New("native definition contains text outside its root")
					}
				}
			}
			if roots != 1 || depth != 0 {
				return Native{}, errors.New("native definition requires one complete root")
			}
		}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return Native{}, errors.New("native file fingerprint failed")
	}
	hash := sha256.New()
	reader := io.LimitReader(file, maxNativeBytes+1)
	buffer := make([]byte, 1024*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return Native{}, err
		}
		n, readErr := reader.Read(buffer)
		size += int64(n)
		hash.Write(buffer[:n])
		if size > maxNativeBytes {
			return Native{}, errors.New("native file exceeds 1 GiB")
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Native{}, errors.New("native file fingerprint failed")
		}
	}
	after, err := file.Stat()
	if err != nil || size != info.Size() || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return Native{}, errors.New("native file changed during validation; retry")
	}
	fingerprint := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	result := Native{Path: path, Filename: filepath.Base(path), Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Size: info.Size(), Fingerprint: fingerprint}
	if isHyper {
		result.CompositionStatus = "ordinary"
	} else if kind == "datasource" {
		result.CompositionStatus, result.ParentDataSourceURLs = classifyDatasourcePackage("definition.tds", definition)
	}
	return result, nil
}
