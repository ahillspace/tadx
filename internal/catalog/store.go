// Package catalog defines the frozen local normalized catalog search contract.
package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLimit              = 20
	maxLimit                  = 100
	maxGenerationBytes        = 64 << 20
	maxGenerationRecords      = 100_000
	maxFieldBytes             = 64 << 10
	maxFilenameComponentBytes = 255
	staleAfter                = 12 * time.Hour
)

// Record is one normalized catalog identity projection.
type Record struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// Generation is one complete, immutable catalog generation.
type Generation struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	Site        string    `json:"site"`
	GeneratedAt time.Time `json:"generated_at"`
	Complete    bool      `json:"complete"`
	Records     []Record  `json:"records"`
}

type generationDocument struct {
	ID          string             `json:"id"`
	Environment string             `json:"environment"`
	Site        *string            `json:"site"`
	GeneratedAt time.Time          `json:"generated_at"`
	Complete    bool               `json:"complete"`
	Records     *boundedRecordList `json:"records"`
}

type boundedRecordList []Record

func (records *boundedRecordList) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
		return errors.New("catalog generation records must be an array")
	}
	decoded := make([]Record, 0)
	for decoder.More() {
		if len(decoded) == maxGenerationRecords {
			return fmt.Errorf("catalog generation exceeds %d-record limit", maxGenerationRecords)
		}
		var record Record
		if err := decoder.Decode(&record); err != nil {
			return err
		}
		decoded = append(decoded, record)
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	*records = decoded
	return nil
}

type invalidCursorError struct{}

func (invalidCursorError) Error() string { return "catalog search cursor is invalid" }

func (invalidCursorError) InvalidCatalogCursor() bool { return true }

// Query contains exact filters plus an optional text search.
type Query struct {
	Text         string
	Kind         string
	ProjectPath  string
	Owner        string
	Environment  string
	Site         string
	SiteSelected bool
	LUID         string
	Cursor       string
	Limit        int
}

// Page is the bounded local continuation envelope.
type Page struct {
	Returned   int
	Total      int
	Limit      int
	NextCursor string
}

// SearchResult includes source generation and staleness provenance.
type SearchResult struct {
	Page         Page
	GenerationID string
	Environment  string
	Site         string
	GeneratedAt  time.Time
	Stale        bool
	Records      []Record
	Warnings     []string
}

// FileStore reads complete normalized generations from portable environment filenames under <root>/catalog.
type FileStore struct {
	root string
	now  func() time.Time
}

// NewFileStore creates a local catalog reader.
func NewFileStore(root string, now func() time.Time) *FileStore {
	if now == nil {
		now = time.Now
	}
	return &FileStore{root: root, now: now}
}

// Search returns a deterministic bounded slice from one complete generation.
func (s *FileStore) Search(ctx context.Context, query Query) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if query.Environment == "" {
		return SearchResult{}, errors.New("catalog search requires an environment")
	}
	if !query.SiteSelected {
		return SearchResult{}, errors.New("catalog search requires a resolved source site")
	}
	filename, err := GenerationFilename(query.Environment)
	if err != nil {
		return SearchResult{}, err
	}
	path := filepath.Join(s.root, "catalog", filename)
	data, err := readGeneration(path)
	if err != nil {
		return SearchResult{}, fmt.Errorf("read catalog generation for environment %q: %w", query.Environment, err)
	}
	var document generationDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return SearchResult{}, fmt.Errorf("decode catalog generation: %w", err)
	}
	generation := Generation{
		ID: document.ID, Environment: document.Environment, GeneratedAt: document.GeneratedAt,
		Complete: document.Complete,
	}
	if document.Site != nil {
		generation.Site = *document.Site
	}
	if !generation.Complete {
		return SearchResult{}, fmt.Errorf("catalog generation %q is incomplete", generation.ID)
	}
	if strings.TrimSpace(generation.ID) == "" {
		return SearchResult{}, errors.New("catalog generation ID is required")
	}
	if err := validateField("generation ID", generation.ID); err != nil {
		return SearchResult{}, err
	}
	if generation.GeneratedAt.IsZero() {
		return SearchResult{}, fmt.Errorf("catalog generation %q generation time is required", generation.ID)
	}
	if err := validateField("source environment", generation.Environment); err != nil {
		return SearchResult{}, err
	}
	if generation.Environment != query.Environment {
		return SearchResult{}, fmt.Errorf("catalog source environment %q does not match selected environment %q", generation.Environment, query.Environment)
	}
	if document.Site == nil {
		return SearchResult{}, fmt.Errorf("catalog generation %q source site is required", generation.ID)
	}
	if err := validateField("source site", generation.Site); err != nil {
		return SearchResult{}, err
	}
	if generation.Site != query.Site {
		return SearchResult{}, fmt.Errorf("catalog source site %q does not match selected site %q", generation.Site, query.Site)
	}
	if document.Records == nil {
		return SearchResult{}, fmt.Errorf("catalog generation %q records are required", generation.ID)
	}
	generation.Records = []Record(*document.Records)
	if err := validateRecords(generation); err != nil {
		return SearchResult{}, err
	}
	items := make([]Record, 0, len(generation.Records))
	text := strings.ToLower(query.Text)
	for _, record := range generation.Records {
		if query.LUID != "" && record.LUID != query.LUID {
			continue
		}
		if query.Kind != "" && record.Kind != query.Kind {
			continue
		}
		if query.ProjectPath != "" && record.ProjectPath != query.ProjectPath {
			continue
		}
		if query.Owner != "" && record.Owner != query.Owner {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(record.Name+" "+record.ProjectPath+" "+record.Owner+" "+record.LUID), text) {
			continue
		}
		items = append(items, record)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		if items[i].ProjectPath != items[j].ProjectPath {
			return items[i].ProjectPath < items[j].ProjectPath
		}
		return items[i].LUID < items[j].LUID
	})
	limit := query.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return SearchResult{}, fmt.Errorf("catalog search limit must be between 1 and %d", maxLimit)
	}
	offset := 0
	if query.Cursor != "" {
		cursorGeneration, cursorQuery, cursorOffset, cursorErr := decodeCursor(query.Cursor)
		if cursorErr != nil || cursorGeneration != generation.ID || cursorQuery != queryFingerprint(query) || cursorOffset < 0 || cursorOffset > len(items) {
			return SearchResult{}, invalidCursorError{}
		}
		offset = cursorOffset
	}
	end := min(offset+limit, len(items))
	pageItems := append([]Record(nil), items[offset:end]...)
	next := ""
	if end < len(items) {
		next = encodeCursor(generation.ID, queryFingerprint(query), end)
	}
	stale := s.now().Sub(generation.GeneratedAt) > staleAfter
	var warnings []string
	if stale {
		warnings = []string{fmt.Sprintf("catalog generation %q is older than 12 hours", generation.ID)}
	}
	return SearchResult{
		Page:         Page{Returned: len(pageItems), Total: len(items), Limit: limit, NextCursor: next},
		GenerationID: generation.ID, Environment: generation.Environment, Site: generation.Site, GeneratedAt: generation.GeneratedAt, Stale: stale,
		Records: pageItems, Warnings: warnings,
	}, nil
}

func readGeneration(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maxGenerationBytes {
		return nil, fmt.Errorf("catalog generation exceeds %d-byte limit", maxGenerationBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxGenerationBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxGenerationBytes {
		return nil, fmt.Errorf("catalog generation exceeds %d-byte limit", maxGenerationBytes)
	}
	return data, nil
}

func validateRecords(generation Generation) error {
	if len(generation.Records) > maxGenerationRecords {
		return fmt.Errorf("catalog generation %q exceeds %d-record limit", generation.ID, maxGenerationRecords)
	}
	seen := make(map[string]struct{}, len(generation.Records))
	for index, record := range generation.Records {
		if strings.TrimSpace(record.LUID) == "" {
			return fmt.Errorf("catalog generation %q record %d LUID is required", generation.ID, index)
		}
		if strings.TrimSpace(record.Kind) == "" {
			return fmt.Errorf("catalog generation %q record %d kind is required", generation.ID, index)
		}
		if strings.TrimSpace(record.Name) == "" {
			return fmt.Errorf("catalog generation %q record %d name is required", generation.ID, index)
		}
		fields := []struct {
			name  string
			value string
		}{
			{name: "LUID", value: record.LUID},
			{name: "kind", value: record.Kind},
			{name: "name", value: record.Name},
			{name: "project path", value: record.ProjectPath},
			{name: "owner", value: record.Owner},
		}
		for _, field := range fields {
			if err := validateField(fmt.Sprintf("catalog generation %q record %d %s", generation.ID, index, field.name), field.value); err != nil {
				return err
			}
		}
		if _, exists := seen[record.LUID]; exists {
			return fmt.Errorf("catalog generation %q contains duplicate LUID %q", generation.ID, record.LUID)
		}
		seen[record.LUID] = struct{}{}
	}
	return nil
}

func validateField(name, value string) error {
	if len(value) > maxFieldBytes {
		return fmt.Errorf("%s exceeds %d-byte limit", name, maxFieldBytes)
	}
	return nil
}

func queryFingerprint(query Query) string {
	value := struct {
		Text        string `json:"text"`
		Kind        string `json:"kind"`
		ProjectPath string `json:"project_path"`
		Owner       string `json:"owner"`
		Environment string `json:"environment"`
		Site        string `json:"site"`
		LUID        string `json:"luid"`
	}{
		Text: strings.ToLower(query.Text), Kind: query.Kind, ProjectPath: query.ProjectPath,
		Owner: query.Owner, Environment: query.Environment, Site: query.Site, LUID: query.LUID,
	}
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func encodeCursor(generationID, query string, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(generationID)) + "." + query + "." + strconv.Itoa(offset)
}

func decodeCursor(value string) (string, string, int, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", 0, errors.New("invalid cursor")
	}
	generationBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", 0, err
	}
	queryBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(queryBytes) != sha256.Size {
		return "", "", 0, errors.New("invalid cursor")
	}
	offset, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", "", 0, err
	}
	return string(generationBytes), parts[1], offset, nil
}

// GenerationFilename returns the portable catalog filename for an exact environment alias.
func GenerationFilename(environment string) (string, error) {
	if environment == "" {
		return "", errors.New("catalog generation requires an environment")
	}
	if isPortableEnvironmentFilename(environment) {
		return environment + ".json", nil
	}
	digest := sha256.Sum256([]byte(environment))
	return "~" + hex.EncodeToString(digest[:]) + ".json", nil
}

func isPortableEnvironmentFilename(environment string) bool {
	if len(environment)+len(".json") > maxFilenameComponentBytes {
		return false
	}
	for _, character := range environment {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	switch environment {
	case "con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return false
	default:
		return true
	}
}
