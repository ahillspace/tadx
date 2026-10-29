package artifact

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WorkbookDatasource is one live published datasource connection in a workbook
// definition. ContentURL is the connection's dbname, which Tableau resolves
// against the destination site when the workbook is published.
type WorkbookDatasource struct {
	Caption    string `json:"caption,omitempty"`
	ContentURL string `json:"content_url"`
}

// WorkbookDatasourceRewrite rebinds the live published datasource connections
// whose content URL is a ContentURLs key to the mapped destination content URL.
type WorkbookDatasourceRewrite struct {
	Path, Fingerprint  string
	Size               int64
	ServerOrigin, Site string
	ContentURLs        map[string]string
}

// RewrittenWorkbook is a temporary upload copy beside its source. The caller
// removes Path once the copy has been read.
type RewrittenWorkbook struct {
	Path, Fingerprint string
	Size              int64
}

// ReadWorkbookDatasources reports the live published datasource connections in
// a downloaded .twb or .twbx payload.
func ReadWorkbookDatasources(ctx context.Context, filename string, content []byte) ([]WorkbookDatasource, error) {
	return readWorkbookDatasources(ctx, bytes.NewReader(content), int64(len(content)), filename)
}

func readWorkbookDatasourceFile(ctx context.Context, path string) ([]WorkbookDatasource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	return readWorkbookDatasources(ctx, file, info.Size(), path)
}

func readWorkbookDatasources(ctx context.Context, source io.ReaderAt, size int64, filename string) ([]WorkbookDatasource, error) {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".twb":
		return walkWorkbookDefinition(ctx, io.NewSectionReader(source, 0, size), nil, nil)
	case ".twbx":
		archive, err := zip.NewReader(source, size)
		if err != nil {
			return nil, errors.New("workbook package could not be read")
		}
		entry, err := primaryPackageEntry(archive, "workbook")
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, errors.New("workbook package has no top-level workbook definition")
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, errors.New("workbook definition could not be read")
		}
		defer stream.Close()
		return walkWorkbookDefinition(ctx, stream, nil, nil)
	}
	return nil, errors.New("workbook datasources require a .twb or .twbx file")
}

// RewriteWorkbookDatasources writes a rebound upload copy of one exact workbook
// file. The source is never modified. For each mapped datasource it rewrites the
// live connection dbname, the connection endpoint when the destination host
// differs, the live repository location, and the dbname in the datasource's
// cached copy, so the uploaded definition stays internally consistent.
func RewriteWorkbookDatasources(ctx context.Context, request WorkbookDatasourceRewrite) (RewrittenWorkbook, error) {
	target, err := newDatasourceRebinding(request)
	if err != nil {
		return RewrittenWorkbook{}, err
	}
	file, err := os.Open(request.Path)
	if err != nil {
		return RewrittenWorkbook{}, errors.New("workbook file could not be opened")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != request.Size {
		return RewrittenWorkbook{}, errors.New("workbook file changed after it was validated")
	}
	fingerprint, err := fingerprintReader(ctx, io.NewSectionReader(file, 0, info.Size()))
	if err != nil {
		return RewrittenWorkbook{}, fmt.Errorf("fingerprint workbook file: %w", err)
	}
	if fingerprint != request.Fingerprint {
		return RewrittenWorkbook{}, errors.New("workbook file changed after it was validated")
	}
	extension := strings.ToLower(filepath.Ext(request.Path))
	if extension != ".twb" && extension != ".twbx" {
		return RewrittenWorkbook{}, errors.New("workbook datasources require a .twb or .twbx file")
	}
	output, err := os.CreateTemp(filepath.Dir(request.Path), ".tadx-publish-rewrite-*"+extension)
	if err != nil {
		return RewrittenWorkbook{}, fmt.Errorf("create workbook upload copy: %w", err)
	}
	result := RewrittenWorkbook{Path: output.Name()}
	keep := false
	defer func() {
		if !keep {
			output.Close()
			os.Remove(result.Path)
		}
	}()
	hash := sha256.New()
	counter := &countingWriter{}
	buffered := bufio.NewWriter(io.MultiWriter(output, hash, counter))
	var seen []WorkbookDatasource
	if extension == ".twb" {
		seen, err = walkWorkbookDefinition(ctx, io.NewSectionReader(file, 0, info.Size()), buffered, target)
	} else {
		seen, err = rewriteWorkbookPackage(ctx, file, info.Size(), buffered, target)
	}
	if err != nil {
		return RewrittenWorkbook{}, err
	}
	if err := buffered.Flush(); err != nil {
		return RewrittenWorkbook{}, fmt.Errorf("write workbook upload copy: %w", err)
	}
	if err := output.Close(); err != nil {
		return RewrittenWorkbook{}, fmt.Errorf("write workbook upload copy: %w", err)
	}
	found := map[string]bool{}
	for _, datasource := range seen {
		found[datasource.ContentURL] = true
	}
	for source := range request.ContentURLs {
		if !found[source] {
			return RewrittenWorkbook{}, fmt.Errorf("workbook has no published datasource connection with content URL %q", source)
		}
	}
	after, err := file.Stat()
	if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return RewrittenWorkbook{}, errors.New("workbook file changed during rewrite; retry")
	}
	keep = true
	result.Size, result.Fingerprint = counter.n, "sha256:"+hex.EncodeToString(hash.Sum(nil))
	return result, nil
}

func rewriteWorkbookPackage(ctx context.Context, source io.ReaderAt, size int64, output io.Writer, target *datasourceRebinding) ([]WorkbookDatasource, error) {
	archive, err := zip.NewReader(source, size)
	if err != nil {
		return nil, errors.New("workbook package could not be read")
	}
	selected, err := primaryPackageEntry(archive, "workbook")
	if err != nil {
		return nil, err
	}
	if selected == nil {
		return nil, errors.New("workbook package has no top-level workbook definition")
	}
	writer := zip.NewWriter(output)
	var seen []WorkbookDatasource
	for _, entry := range archive.File {
		if entry != selected {
			if err := writer.Copy(entry); err != nil {
				return nil, fmt.Errorf("copy workbook package entry %q: %w", entry.Name, err)
			}
			continue
		}
		header := entry.FileHeader
		header.CRC32, header.CompressedSize, header.UncompressedSize, header.CompressedSize64, header.UncompressedSize64, header.Extra = 0, 0, 0, 0, 0, nil
		definition, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, fmt.Errorf("write workbook definition: %w", err)
		}
		stream, err := entry.Open()
		if err != nil {
			return nil, errors.New("workbook definition could not be read")
		}
		seen, err = walkWorkbookDefinition(ctx, stream, definition, target)
		closeErr := stream.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, errors.New("workbook definition could not be read")
		}
	}
	if err := writer.SetComment(archive.Comment); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("write workbook package: %w", err)
	}
	return seen, nil
}

// datasourceRebinding holds the destination facts applied to mapped datasources.
type datasourceRebinding struct {
	contentURLs                  map[string]string
	origin, host, port, location string
	site                         string
}

func newDatasourceRebinding(request WorkbookDatasourceRewrite) (*datasourceRebinding, error) {
	if len(request.ContentURLs) == 0 {
		return nil, errors.New("workbook rewrite requires at least one datasource binding")
	}
	for source, destination := range request.ContentURLs {
		if source == "" || destination == "" || strings.Contains(destination, "]]>") {
			return nil, fmt.Errorf("invalid datasource binding %q to %q", source, destination)
		}
	}
	parsed, err := url.Parse(request.ServerOrigin)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Hostname() == "" || parsed.User != nil || strings.Trim(parsed.Path, "/") != "" {
		return nil, errors.New("workbook rewrite requires an absolute HTTPS server origin")
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	location := "/datasources"
	if request.Site != "" {
		location = "/t/" + request.Site + "/datasources"
	}
	origin := (&url.URL{Scheme: "https", Host: strings.ToLower(parsed.Host)}).String()
	return &datasourceRebinding{contentURLs: request.ContentURLs, origin: origin, host: strings.ToLower(parsed.Hostname()), port: port, location: location, site: request.Site}, nil
}

// connectionEdits rebinds a live connection and moves its endpoint to the
// destination host only when the workbook was bound to another host.
func (r *datasourceRebinding) connectionEdits(destination string, attributes []xml.Attr) []attributeEdit {
	edits := []attributeEdit{{name: "dbname", value: destination}}
	if !strings.EqualFold(attributeValue(attributes, "server"), r.host) {
		edits = append(edits, attributeEdit{name: "channel", value: "https"}, attributeEdit{name: "port", value: r.port}, attributeEdit{name: "server", value: r.host})
	}
	return edits
}

func (r *datasourceRebinding) locationEdits(destination string, attributes []xml.Attr) []attributeEdit {
	query := ""
	if derived := attributeValue(attributes, "derived-from"); strings.Contains(derived, "?") {
		query = derived[strings.Index(derived, "?"):]
	}
	return []attributeEdit{
		{name: "derived-from", value: r.origin + r.location + "/" + destination + query},
		{name: "id", value: destination},
		{name: "path", value: r.location},
		{name: "site", value: r.site, remove: r.site == ""},
	}
}

// segment is one span of the definition. Spans inside a live datasource are
// held until its connection decides whether the datasource is rebound.
type segment struct {
	raw        []byte
	kind       int
	attributes []xml.Attr
}

const (
	segmentPlain = iota
	segmentLocation
	segmentCache
)

// walkWorkbookDefinition streams a workbook definition, reporting each live
// published datasource connection: a sqlproxy connection directly inside a
// datasource directly inside /workbook/datasources. Worksheet datasource
// references and embedded connections are not live published connections.
// When output is non-nil, it copies every byte unchanged except the spans of
// datasources whose content URL target maps.
func walkWorkbookDefinition(ctx context.Context, source io.Reader, output io.Writer, target *datasourceRebinding) ([]WorkbookDatasource, error) {
	recorder := &recordingReader{reader: bufio.NewReader(source)}
	decoder := xml.NewDecoder(recorder)
	var (
		stack       []string
		datasources []WorkbookDatasource
		consumed    int64
		live        bool
		liveDepth   int
		caption     string
		decided     bool
		destination string
		held        []segment
		cacheDepth  int
		tokens      int
	)
	emit := func(part segment) error {
		if output == nil {
			return nil
		}
		if live && !decided {
			part.raw = bytes.Clone(part.raw)
			held = append(held, part)
			return nil
		}
		return writeSegment(output, part, destination, target)
	}
	flush := func() error {
		for _, part := range held {
			if err := writeSegment(output, part, destination, target); err != nil {
				return err
			}
		}
		held = held[:0]
		return nil
	}
	for {
		tokens++
		if tokens%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("workbook definition contains invalid XML")
		}
		end := decoder.InputOffset()
		raw := recorder.take(end - consumed)
		consumed = end
		part := segment{raw: raw}
		switch token := token.(type) {
		case xml.StartElement:
			name := token.Name.Local
			if len(stack) == 0 && name != "workbook" {
				return nil, errors.New("workbook definition requires a workbook root")
			}
			depth := len(stack)
			stack = append(stack, name)
			if !live && name == "datasource" && depth == 2 && stack[1] == "datasources" {
				live, liveDepth, caption, decided, destination = true, depth, attributeValue(token.Attr, "caption"), false, ""
				break
			}
			if !live {
				break
			}
			switch {
			case depth == liveDepth+1 && name == "connection" && !decided:
				// The datasource's own connection decides whether it is a live
				// published connection; held spans are released either way.
				decided = true
				if attributeValue(token.Attr, "class") == "sqlproxy" {
					contentURL := attributeValue(token.Attr, "dbname")
					datasources = append(datasources, WorkbookDatasource{Caption: caption, ContentURL: contentURL})
					if target != nil {
						destination = target.contentURLs[contentURL]
					}
				}
				if output == nil {
					break
				}
				if err := flush(); err != nil {
					return nil, err
				}
				if destination != "" {
					rewritten, err := rewriteStartTag(raw, target.connectionEdits(destination, token.Attr))
					if err != nil {
						return nil, err
					}
					part.raw = rewritten
				}
			case depth == liveDepth+1 && name == "repository-location":
				part.kind, part.attributes = segmentLocation, token.Attr
			case name == "attribute" && attributeValue(token.Attr, "name") == "datasource" && cacheDepth == 0:
				cacheDepth = len(stack)
			}
		case xml.EndElement:
			if live && len(stack)-1 == liveDepth {
				if err := emit(part); err != nil {
					return nil, err
				}
				decided = true
				if output != nil {
					if err := flush(); err != nil {
						return nil, err
					}
				}
				live, destination = false, ""
				stack = stack[:len(stack)-1]
				continue
			}
			if cacheDepth == len(stack) {
				cacheDepth = 0
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if live && cacheDepth != 0 && bytes.HasPrefix(raw, []byte("<![CDATA[")) {
				part.kind = segmentCache
			}
		}
		if err := emit(part); err != nil {
			return nil, err
		}
	}
	if len(stack) != 0 || consumed == 0 {
		return nil, errors.New("workbook definition requires one complete workbook root")
	}
	if output != nil {
		if _, err := output.Write(recorder.take(recorder.unclaimed())); err != nil {
			return nil, err
		}
	}
	return datasources, nil
}

func writeSegment(output io.Writer, part segment, destination string, target *datasourceRebinding) error {
	raw := part.raw
	if destination != "" {
		var err error
		switch part.kind {
		case segmentLocation:
			raw, err = rewriteStartTag(raw, target.locationEdits(destination, part.attributes))
		case segmentCache:
			raw, err = rewriteCachedDatasource(raw, destination)
		}
		if err != nil {
			return err
		}
	}
	_, err := output.Write(raw)
	return err
}

// rewriteCachedDatasource rebinds the sqlproxy connection dbname inside the
// datasource copy Tableau caches in a CDATA metadata attribute. The cached copy
// carries Tableau-internal endpoints and repository paths, which stay as-is.
func rewriteCachedDatasource(raw []byte, destination string) ([]byte, error) {
	const open, closing = "<![CDATA[", "]]>"
	if !bytes.HasPrefix(raw, []byte(open)) || !bytes.HasSuffix(raw, []byte(closing)) {
		return raw, nil
	}
	content := raw[len(open) : len(raw)-len(closing)]
	decoder := xml.NewDecoder(bytes.NewReader(content))
	var result bytes.Buffer
	result.WriteString(open)
	var copied int64
	for {
		start := decoder.InputOffset()
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A cached copy that is not a parseable datasource is opaque content.
			return raw, nil
		}
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "connection" || attributeValue(element.Attr, "class") != "sqlproxy" {
			continue
		}
		end := decoder.InputOffset()
		rewritten, err := rewriteStartTag(content[start:end], []attributeEdit{{name: "dbname", value: destination}})
		if err != nil {
			return nil, err
		}
		result.Write(content[copied:start])
		result.Write(rewritten)
		copied = end
	}
	result.Write(content[copied:])
	result.WriteString(closing)
	return result.Bytes(), nil
}

type attributeEdit struct {
	name, value string
	remove      bool
}

type attributeSpan struct {
	name                        string
	start, valueStart, valueEnd int
	quote                       byte
}

// rewriteStartTag edits attribute values of one raw start tag in place,
// preserving every other byte. Missing attributes are inserted in the
// alphabetical position Tableau writes them in.
func rewriteStartTag(raw []byte, edits []attributeEdit) ([]byte, error) {
	spans, nameEnd, err := lexStartTag(raw)
	if err != nil {
		return nil, err
	}
	quote := byte('\'')
	if len(spans) > 0 {
		quote = spans[0].quote
	}
	existing := map[string]bool{}
	for _, span := range spans {
		existing[span.name] = true
	}
	byName := map[string]attributeEdit{}
	var missing []attributeEdit
	for _, edit := range edits {
		byName[edit.name] = edit
		if !existing[edit.name] && !edit.remove {
			missing = append(missing, edit)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].name < missing[j].name })
	var result bytes.Buffer
	writeMissing := func(before string) {
		for len(missing) > 0 && (before == "" || missing[0].name < before) {
			fmt.Fprintf(&result, " %s=%c%s%c", missing[0].name, quote, escapeAttribute(missing[0].value, quote), quote)
			missing = missing[1:]
		}
	}
	result.Write(raw[:nameEnd])
	cursor := nameEnd
	for _, span := range spans {
		result.Write(raw[cursor:span.start])
		writeMissing(span.name)
		cursor = span.valueEnd + 1
		edit, edited := byName[span.name]
		switch {
		case !edited:
			result.Write(raw[span.start:cursor])
		case !edit.remove:
			result.Write(raw[span.start:span.valueStart])
			result.WriteString(escapeAttribute(edit.value, span.quote))
			result.WriteByte(span.quote)
		}
	}
	writeMissing("")
	result.Write(raw[cursor:])
	return result.Bytes(), nil
}

// lexStartTag finds the attribute spans of one raw start tag. Each span starts
// at the whitespace preceding its name; end is the offset after the tag name.
func lexStartTag(raw []byte) ([]attributeSpan, int, error) {
	invalid := errors.New("workbook definition contains an unreadable start tag")
	if len(raw) < 2 || raw[0] != '<' {
		return nil, 0, invalid
	}
	index := 1
	for index < len(raw) && !isTagSpace(raw[index]) && raw[index] != '/' && raw[index] != '>' {
		index++
	}
	end := index
	var spans []attributeSpan
	for {
		start := index
		for index < len(raw) && isTagSpace(raw[index]) {
			index++
		}
		if index >= len(raw) {
			return nil, 0, invalid
		}
		if raw[index] == '/' || raw[index] == '>' {
			return spans, end, nil
		}
		nameStart := index
		for index < len(raw) && raw[index] != '=' && !isTagSpace(raw[index]) {
			index++
		}
		name := string(raw[nameStart:index])
		for index < len(raw) && isTagSpace(raw[index]) {
			index++
		}
		if index >= len(raw) || raw[index] != '=' {
			return nil, 0, invalid
		}
		index++
		for index < len(raw) && isTagSpace(raw[index]) {
			index++
		}
		if index >= len(raw) || (raw[index] != '\'' && raw[index] != '"') {
			return nil, 0, invalid
		}
		quote := raw[index]
		valueStart := index + 1
		closing := bytes.IndexByte(raw[valueStart:], quote)
		if closing < 0 {
			return nil, 0, invalid
		}
		index = valueStart + closing
		spans = append(spans, attributeSpan{name: name, start: start, valueStart: valueStart, valueEnd: index, quote: quote})
		index++
	}
}

func isTagSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r'
}

func escapeAttribute(value string, quote byte) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", "'", "&apos;")
	if quote == '"' {
		replacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", `"`, "&quot;")
	}
	return replacer.Replace(value)
}

func attributeValue(attributes []xml.Attr, name string) string {
	for _, attribute := range attributes {
		if attribute.Name.Space == "" && attribute.Name.Local == name {
			return attribute.Value
		}
	}
	return ""
}

// recordingReader retains the bytes the decoder consumes so each token's raw
// span can be copied unchanged. The decoder may hold one byte back, so the
// buffer keeps unclaimed bytes and compacts once the claimed span is used.
type recordingReader struct {
	reader  *bufio.Reader
	pending []byte
	claimed int
}

func (r *recordingReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	value, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	buffer[0] = value
	return 1, nil
}

func (r *recordingReader) ReadByte() (byte, error) {
	if r.claimed > 0 {
		r.pending = r.pending[:copy(r.pending, r.pending[r.claimed:])]
		r.claimed = 0
	}
	value, err := r.reader.ReadByte()
	if err == nil {
		r.pending = append(r.pending, value)
	}
	return value, err
}

// take claims the next count consumed bytes. The result is valid until the
// next read.
func (r *recordingReader) take(count int64) []byte {
	raw := r.pending[r.claimed : r.claimed+int(count)]
	r.claimed += int(count)
	return raw
}

func (r *recordingReader) unclaimed() int64 {
	return int64(len(r.pending) - r.claimed)
}

type countingWriter struct{ n int64 }

func (w *countingWriter) Write(buffer []byte) (int, error) {
	w.n += int64(len(buffer))
	return len(buffer), nil
}
