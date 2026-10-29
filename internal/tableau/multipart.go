package tableau

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"path/filepath"
	"strings"
)

// MultipartPart is one part of a Tableau multipart/mixed request. A part
// carries either inline bytes or a byte range of a file.
type MultipartPart struct {
	header textproto.MIMEHeader
	data   []byte
	file   io.ReaderAt
	offset int64
	size   int64
}

// RequestPayloadPart is the request_payload part that carries the request XML.
func RequestPayloadPart(payload []byte) MultipartPart {
	return MultipartPart{header: textproto.MIMEHeader{"Content-Disposition": {`name="request_payload"`}, "Content-Type": {"text/xml"}}, data: payload}
}

// FilePart sends size bytes of file, starting at offset, as the named file part.
func FilePart(name, filename string, file io.ReaderAt, offset, size int64) MultipartPart {
	formatted := mime.FormatMediaType("form-data", map[string]string{"filename": filepath.Base(filename)})
	disposition := fmt.Sprintf(`name="%s"; %s`, name, strings.TrimPrefix(formatted, "form-data; "))
	return MultipartPart{header: textproto.MIMEHeader{"Content-Disposition": {disposition}, "Content-Type": {"application/octet-stream"}}, file: file, offset: offset, size: size}
}

// MultipartBody is a replayable multipart/mixed request body. Only the part
// framing is buffered; file parts are read from their source on every send,
// so an upload never holds the content in memory and a redirect resends
// identical bytes.
type MultipartBody struct {
	segments    []func() io.Reader
	length      int64
	contentType string
}

// NewMultipartBody encodes parts in order.
func NewMultipartBody(parts ...MultipartPart) (*MultipartBody, error) {
	var framing bytes.Buffer
	writer := multipart.NewWriter(&framing)
	type fileRange struct {
		framingEnd   int
		file         io.ReaderAt
		offset, size int64
	}
	var files []fileRange
	var length int64
	for _, part := range parts {
		content, err := writer.CreatePart(part.header)
		if err != nil {
			return nil, err
		}
		if part.file == nil {
			if _, err := content.Write(part.data); err != nil {
				return nil, err
			}
			continue
		}
		files = append(files, fileRange{framingEnd: framing.Len(), file: part.file, offset: part.offset, size: part.size})
		length += part.size
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	encoded := framing.Bytes()
	body := &MultipartBody{length: length + int64(len(encoded)), contentType: "multipart/mixed; boundary=" + writer.Boundary()}
	start := 0
	for _, file := range files {
		chunk := encoded[start:file.framingEnd]
		body.segments = append(body.segments, func() io.Reader { return bytes.NewReader(chunk) }, func() io.Reader {
			return io.NewSectionReader(file.file, file.offset, file.size)
		})
		start = file.framingEnd
	}
	tail := encoded[start:]
	body.segments = append(body.segments, func() io.Reader { return bytes.NewReader(tail) })
	return body, nil
}

// Len reports the exact encoded size in bytes.
func (b *MultipartBody) Len() int64 { return b.length }

// ContentType is the multipart/mixed media type with its boundary.
func (b *MultipartBody) ContentType() string { return b.contentType }

func (b *MultipartBody) open() io.ReadCloser {
	readers := make([]io.Reader, len(b.segments))
	for index, segment := range b.segments {
		readers[index] = segment()
	}
	return io.NopCloser(io.MultiReader(readers...))
}
