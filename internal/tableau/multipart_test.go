package tableau

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
)

func TestMultipartBodyStreamsFileRangesAndReplaysIdenticalBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.bin")
	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	body, err := NewMultipartBody(RequestPayloadPart([]byte("<tsRequest/>")), FilePart("tableau_file", "dir/Sales Report.twbx", file, 4, 6))
	if err != nil {
		t.Fatal(err)
	}
	first, err := io.ReadAll(body.open())
	if err != nil {
		t.Fatal(err)
	}
	second, err := io.ReadAll(body.open())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || int64(len(first)) != body.Len() {
		t.Fatalf("replay differs or length %d != %d", len(first), body.Len())
	}
	mediaType, params, err := mime.ParseMediaType(body.ContentType())
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("content type = %q, err = %v", body.ContentType(), err)
	}
	reader := multipart.NewReader(bytes.NewReader(first), params["boundary"])
	want := []struct{ disposition, contentType, data string }{
		{`name="request_payload"`, "text/xml", "<tsRequest/>"},
		{`name="tableau_file"; filename="Sales Report.twbx"`, "application/octet-stream", "456789"},
	}
	for _, expected := range want {
		part, err := reader.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		if part.Header.Get("Content-Disposition") != expected.disposition || part.Header.Get("Content-Type") != expected.contentType || string(data) != expected.data {
			t.Fatalf("part = %v %q, want %+v", part.Header, data, expected)
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("trailing part err = %v", err)
	}
}

func TestMultipartBodyMatchesBufferedEncoding(t *testing.T) {
	content := []byte("workbook bytes")
	body, err := NewMultipartBody(FilePart("tableau_workbook", "Finance.twb", bytes.NewReader(content), 0, int64(len(content))))
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := io.ReadAll(body.open())
	if err != nil {
		t.Fatal(err)
	}
	_, params, _ := mime.ParseMediaType(body.ContentType())
	var buffered bytes.Buffer
	writer := multipart.NewWriter(&buffered)
	if err := writer.SetBoundary(params["boundary"]); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreatePart(map[string][]string{"Content-Disposition": {`name="tableau_workbook"; filename=Finance.twb`}, "Content-Type": {"application/octet-stream"}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = writer.Close()
	if !bytes.Equal(streamed, buffered.Bytes()) {
		t.Fatalf("streamed body\n%q\nwant\n%q", streamed, buffered.Bytes())
	}
}
