package tableau

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/ahillspace/tadx/internal/auth"
)

// ErrPublishPayloadChanged reports that a publish payload no longer matches
// the size and fingerprint recorded when the publish was planned.
var ErrPublishPayloadChanged = errors.New("publish payload changed after planning")

// PublishSnapshot is a fingerprint-verified private copy of a publish
// payload. Every publish request reads the snapshot, so the bytes Tableau
// receives are exactly the bytes that were fingerprinted, even if the
// artifact changes after planning. One final publish request claims it.
type PublishSnapshot struct {
	file     *os.File
	size     int64
	mu       sync.Mutex
	claimed  bool
	released bool
	err      error
}

// SnapshotPublishPayload copies the regular file at path once into a
// temporary file beside it, hashing it in the same pass, and fails with
// ErrPublishPayloadChanged when its size or sha256 fingerprint differs from
// the plan.
func SnapshotPublishPayload(ctx context.Context, path string, size int64, fingerprint string) (*PublishSnapshot, error) {
	source, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open publish payload: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect publish payload: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() != size {
		return nil, ErrPublishPayloadChanged
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".tadx-publish-snapshot-*")
	if err != nil {
		return nil, fmt.Errorf("create publish snapshot: %w", err)
	}
	snapshot := &PublishSnapshot{file: file}
	digest := sha256.New()
	copied, err := copyContext(ctx, io.MultiWriter(file, digest), source)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("snapshot publish payload: %w", err), snapshot.Close())
	}
	if copied != size || "sha256:"+hex.EncodeToString(digest.Sum(nil)) != fingerprint {
		return nil, errors.Join(ErrPublishPayloadChanged, snapshot.Close())
	}
	snapshot.size = copied
	return snapshot, nil
}

// File returns the snapshot content for multipart file parts.
func (s *PublishSnapshot) File() io.ReaderAt { return s.file }

// Size reports the snapshot length in bytes.
func (s *PublishSnapshot) Size() int64 { return s.size }

// Claim reserves the snapshot for the one final publish request, so a
// prepared publish is never sent twice or after its snapshot was released.
func (s *PublishSnapshot) Claim() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed || s.released {
		return errors.New("publish was already sent or released")
	}
	s.claimed = true
	return nil
}

// Close removes the snapshot. Calls after the first return its result.
func (s *PublishSnapshot) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.released {
		s.released = true
		s.err = errors.Join(s.file.Close(), os.Remove(s.file.Name()))
	}
	return s.err
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 1024*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

// uploadResponseLimit bounds the small XML confirmation that file upload
// initiate and append requests return.
const uploadResponseLimit = 1024 * 1024

// FileUpload sends one snapshot through a Tableau file upload session.
type FileUpload struct {
	Transport *Transport
	Session   auth.Session
	ServerURL string
	// Operation prefixes the initiate and append operation names, such as
	// "workbook" for workbook.upload.initiate.
	Operation string
	Filename  string
	ChunkSize int64
}

// Upload initiates a file upload session, appends the snapshot in ChunkSize
// blocks, and returns the session ID the publish request references. REST API
// 3.27 and later receive an explicit sequenceID for each block.
func (u FileUpload) Upload(ctx context.Context, snapshot *PublishSnapshot) (string, error) {
	ordered, err := APIVersionAtLeast(u.Transport.APIVersion(), 3, 27)
	if err != nil {
		return "", fmt.Errorf("interpret Tableau REST API version for chunked upload: %w", err)
	}
	initiateOperation, appendOperation := u.Operation+".upload.initiate", u.Operation+".upload.append"
	response, err := u.Transport.Do(ctx, u.Session, Request{Method: http.MethodPost, ServerURL: u.ServerURL, Path: u.path(), Accept: "application/xml", Operation: initiateOperation, MaxResponseBytes: uploadResponseLimit})
	if err != nil {
		return "", err
	}
	sessionID, err := uploadSessionID(response, http.StatusCreated)
	if err != nil {
		return "", NewProtocolError(initiateOperation, response, fmt.Errorf("initiate file upload: %w", err), false)
	}
	for sequence, offset := 1, int64(0); offset < snapshot.size; sequence++ {
		blockSize := min(snapshot.size-offset, u.ChunkSize)
		body, err := NewMultipartBody(RequestPayloadPart(nil), FilePart("tableau_file", u.Filename, snapshot.file, offset, blockSize))
		if err != nil {
			return "", err
		}
		offset += blockSize
		query := url.Values{}
		if ordered {
			query.Set("sequenceID", strconv.Itoa(sequence))
		}
		response, err := u.Transport.Do(ctx, u.Session, Request{Method: http.MethodPut, ServerURL: u.ServerURL, Path: u.path(sessionID), Query: query, Multipart: body, Accept: "application/xml", Operation: appendOperation, MaxResponseBytes: uploadResponseLimit})
		if err != nil {
			return "", err
		}
		appended, err := uploadSessionID(response, http.StatusOK)
		if err != nil {
			return "", NewProtocolError(appendOperation, response, fmt.Errorf("append file upload: %w", err), false)
		}
		if appended != sessionID {
			return "", NewProtocolError(appendOperation, response, fmt.Errorf("append file upload returned upload session ID %q, expected %q", appended, sessionID), false)
		}
	}
	return sessionID, nil
}

func (u FileUpload) path(sessionID ...string) string {
	segments := append([]string{"api", u.Transport.APIVersion(), "sites", u.Session.SiteLUID(), "fileUploads"}, sessionID...)
	for index := range segments {
		segments[index] = url.PathEscape(segments[index])
	}
	return "/" + strings.Join(segments, "/")
}

func uploadSessionID(response Response, status int) (string, error) {
	if response.StatusCode != status {
		return "", fmt.Errorf("returned HTTP %d, expected %d", response.StatusCode, status)
	}
	var envelope struct {
		Upload struct {
			SessionID string `xml:"uploadSessionId,attr"`
		} `xml:"fileUpload"`
	}
	if err := xml.Unmarshal(response.Body, &envelope); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if envelope.Upload.SessionID == "" {
		return "", errors.New("response omitted the upload session ID")
	}
	return envelope.Upload.SessionID, nil
}

// APIVersionAtLeast reports whether a major.minor Tableau REST API version is
// at least major.minor.
func APIVersionAtLeast(version string, major, minor int) (bool, error) {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false, fmt.Errorf("malformed Tableau REST API version %q", version)
	}
	gotMajor, majorErr := strconv.Atoi(parts[0])
	gotMinor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false, fmt.Errorf("malformed Tableau REST API version %q", version)
	}
	return gotMajor > major || (gotMajor == major && gotMinor >= minor), nil
}
