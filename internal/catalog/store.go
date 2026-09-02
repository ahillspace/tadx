// Package catalog stores immutable Tableau catalog generations in SQLite.
package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultLimit         = 20
	maxLimit             = 100
	maxBatchRows         = 10_000
	maxFieldBytes        = 64 << 10
	staleAfter           = 12 * time.Hour
	schemaVersion        = 1
	databaseRelativePath = "catalog/catalog.sqlite"
)

var publicScopes = []string{"users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions"}

var batchColumns = map[string][]string{
	"users":       {"id", "name", "email", "site_role", "last_login"},
	"groups":      {"id", "name", "domain"},
	"projects":    {"id", "name", "parent_project_id", "description", "owner_id"},
	"workbooks":   {"id", "name", "project_id", "owner_id", "size", "updated_at"},
	"datasources": {"id", "name", "project_id", "owner_id", "updated_at"},
	"flows":       {"id", "name", "project_id", "owner_id", "updated_at"},
	"views":       {"id", "name", "workbook_id"},
	"permissions": {"content_type", "content_id", "grantee_type", "grantee_id", "capability", "mode"},
}

type Record struct {
	LUID        string `json:"luid"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ProjectPath string `json:"project_path,omitempty"`
	Owner       string `json:"owner,omitempty"`
}
type Generation struct {
	ID, Environment, Site    string
	GeneratedAt              time.Time
	Complete                 bool
	Source                   string
	Scopes, DependencyScopes []string
	Records                  []Record
}
type GenerationMetadata struct {
	ID, Environment, Site           string
	GeneratedAt                     time.Time
	Source                          string
	RequestedScopes, ImplicitScopes []string
}
type Batch struct {
	Scope   string
	Columns []string
	Rows    [][]any
}
type Query struct {
	Text, Kind, Name, ProjectPath, Owner, Environment, Site string
	SiteSelected                                            bool
	LUID, Cursor                                            string
	Limit                                                   int
}
type Page struct {
	Returned, Total, Limit int
	NextCursor             string
}
type SearchResult struct {
	Page                            Page
	GenerationID, Environment, Site string
	GeneratedAt                     time.Time
	Stale                           bool
	Source                          string
	Records                         []Record
	Warnings                        []string
}
type Selection struct {
	Environment, Site string
	SiteSelected      bool
}
type Lookup struct {
	Environment, Site             string
	SiteSelected                  bool
	LUID, Kind, Name, ProjectPath string
}
type GetResult struct {
	Record                          Record
	GenerationID, Environment, Site string
	GeneratedAt                     time.Time
	Stale                           bool
	Warnings                        []string
}
type StatusResult struct {
	GenerationID, Environment, Site string
	GeneratedAt                     time.Time
	Age                             time.Duration
	Complete, Stale                 bool
	Source, Path                    string
	RecordCount                     int
	Warnings                        []string
}
type ReplaceResult struct {
	GenerationID, Path string
	RecordCount        int
}

type invalidCursorError struct{}

func (invalidCursorError) Error() string              { return "catalog search cursor is invalid" }
func (invalidCursorError) InvalidCatalogCursor() bool { return true }

type ambiguousSelectorError struct{}

func (ambiguousSelectorError) Error() string                  { return "catalog selector is ambiguous" }
func (ambiguousSelectorError) AmbiguousCatalogSelector() bool { return true }

type notFoundError struct{}

func (notFoundError) Error() string               { return "catalog record was not found" }
func (notFoundError) CatalogRecordNotFound() bool { return true }

type unavailableScopeError struct{ scope string }

func (e unavailableScopeError) Error() string {
	return fmt.Sprintf("catalog scope %q is not present in the current generation", e.scope)
}
func (unavailableScopeError) CatalogScopeUnavailable() bool { return true }

// Store owns one config-root SQLite catalog database.
type Store struct {
	root   string
	now    func() time.Time
	initMu sync.Mutex
}

// NewStore creates a catalog store.
func NewStore(root string, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{root: root, now: now}
}
func DatabasePath() string { return databaseRelativePath }
func (s *Store) databasePath() string {
	return filepath.Join(s.root, filepath.FromSlash(databaseRelativePath))
}

func (s *Store) open(ctx context.Context) (*sql.DB, error) {
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(s.databasePath()), 0o700); err != nil {
		return nil, fmt.Errorf("create catalog directory: %w", err)
	}
	path := s.databasePath()
	newDatabase := false
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		newDatabase = true
	} else if err != nil {
		return nil, fmt.Errorf("inspect catalog database: %w", err)
	} else if !info.Mode().IsRegular() {
		return nil, errors.New("catalog database must be a regular file")
	}
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open catalog database: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if newDatabase {
		if err := initializeSchema(ctx, db); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := validateSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func (s *Store) BeginGeneration(ctx context.Context, metadata GenerationMetadata) (*GenerationWriter, error) {
	metadata.RequestedScopes = normalizedScopes(metadata.RequestedScopes, true)
	metadata.ImplicitScopes = normalizedScopes(metadata.ImplicitScopes, false)
	if err := validateMetadata(metadata); err != nil {
		return nil, err
	}
	db, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("begin catalog generation: %w", err)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO generations(id,fingerprint,environment,site,generated_at,complete,source,record_count,created_at) VALUES(NULL,NULL,?,?,?,0,?,0,?)`, metadata.Environment, metadata.Site, metadata.GeneratedAt.UTC().Format(time.RFC3339Nano), metadata.Source, s.now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("stage catalog generation: %w", err)
	}
	key, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		db.Close()
		return nil, err
	}
	requested := map[string]bool{}
	for _, scope := range metadata.RequestedScopes {
		requested[scope] = true
	}
	seen := map[string]bool{}
	for _, scope := range append(append([]string{}, metadata.RequestedScopes...), metadata.ImplicitScopes...) {
		if seen[scope] {
			continue
		}
		seen[scope] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO generation_scopes(generation_key,scope,requested,complete) VALUES(?,?,?,0)`, key, scope, requested[scope]); err != nil {
			tx.Rollback()
			db.Close()
			return nil, err
		}
	}
	return &GenerationWriter{store: s, db: db, tx: tx, key: key, metadata: metadata}, nil
}

func (s *Store) Replace(ctx context.Context, generation Generation) (ReplaceResult, error) {
	if !generation.Complete {
		return ReplaceResult{}, errors.New("catalog replacement generation must be complete")
	}
	if generation.Records == nil {
		return ReplaceResult{}, errors.New("catalog replacement records are required")
	}
	w, err := s.BeginGeneration(ctx, GenerationMetadata{ID: generation.ID, Environment: generation.Environment, Site: generation.Site, GeneratedAt: generation.GeneratedAt, Source: generation.Source, RequestedScopes: generation.Scopes, ImplicitScopes: generation.DependencyScopes})
	if err != nil {
		return ReplaceResult{}, err
	}
	defer w.Rollback()
	if err := w.writeRecords(ctx, generation.Records); err != nil {
		return ReplaceResult{}, err
	}
	if err := w.CompleteScopes(ctx, append(append([]string{}, w.metadata.RequestedScopes...), w.metadata.ImplicitScopes...)); err != nil {
		return ReplaceResult{}, err
	}
	return w.Publish(ctx)
}

func (s *Store) Search(ctx context.Context, query Query) (SearchResult, error) {
	if err := validateSelection(query.Environment, query.SiteSelected); err != nil {
		return SearchResult{}, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return SearchResult{}, fmt.Errorf("catalog search limit must be between 1 and %d", maxLimit)
	}
	query.Limit = limit
	db, err := s.open(ctx)
	if err != nil {
		return SearchResult{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return SearchResult{}, err
	}
	defer tx.Rollback()
	meta, err := currentGeneration(ctx, tx, query.Environment, query.Site)
	if err != nil {
		return SearchResult{}, err
	}
	if query.Kind != "" {
		if scope := kindScope(query.Kind); scope != "" {
			var requested bool
			err := tx.QueryRowContext(ctx, `SELECT requested FROM generation_scopes WHERE generation_key=? AND scope=?`, meta.key, scope).Scan(&requested)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && !requested) {
				return SearchResult{}, unavailableScopeError{scope}
			}
			if err != nil {
				return SearchResult{}, err
			}
		}
	}
	where, args := searchWhere(meta.key, query)
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM catalog_records `+where, args...).Scan(&total); err != nil {
		return SearchResult{}, fmt.Errorf("count catalog records: %w", err)
	}
	offset := 0
	if query.Cursor != "" {
		id, fingerprint, decoded, decodeErr := decodeCursor(query.Cursor)
		if decodeErr != nil || id != meta.id || fingerprint != queryFingerprint(query) || decoded < 0 || decoded > total {
			return SearchResult{}, invalidCursorError{}
		}
		offset = decoded
	}
	rows, err := tx.QueryContext(ctx, `SELECT luid,kind,name,project_path,owner FROM catalog_records `+where+` ORDER BY kind,name,project_path,luid LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return SearchResult{}, err
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.LUID, &r.Kind, &r.Name, &r.ProjectPath, &r.Owner); err != nil {
			return SearchResult{}, err
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return SearchResult{}, err
	}
	end := offset + len(records)
	next := ""
	if end < total {
		next = encodeCursor(meta.id, queryFingerprint(query), end)
	}
	stale, warnings := staleness(s.now, meta.generatedAt, meta.id)
	if err := tx.Commit(); err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Page: Page{len(records), total, limit, next}, GenerationID: meta.id, Environment: meta.environment, Site: meta.site, GeneratedAt: meta.generatedAt, Stale: stale, Source: meta.source, Records: records, Warnings: warnings}, nil
}

func (s *Store) Get(ctx context.Context, lookup Lookup) (GetResult, error) {
	if lookup.LUID == "" && (lookup.Kind == "" || lookup.Name == "") {
		return GetResult{}, errors.New("catalog get requires a LUID or exact kind and name")
	}
	q := Query{Environment: lookup.Environment, Site: lookup.Site, SiteSelected: lookup.SiteSelected, Limit: 2}
	if lookup.LUID != "" {
		q.LUID = lookup.LUID
	} else {
		q.Kind = lookup.Kind
		q.Name = lookup.Name
		q.ProjectPath = lookup.ProjectPath
	}
	r, err := s.Search(ctx, q)
	if err != nil {
		return GetResult{}, err
	}
	if r.Page.Total > 1 {
		return GetResult{}, ambiguousSelectorError{}
	}
	if len(r.Records) == 0 {
		return GetResult{}, notFoundError{}
	}
	return GetResult{r.Records[0], r.GenerationID, r.Environment, r.Site, r.GeneratedAt, r.Stale, append([]string(nil), r.Warnings...)}, nil
}
func (s *Store) Status(ctx context.Context, selection Selection) (StatusResult, error) {
	if err := validateSelection(selection.Environment, selection.SiteSelected); err != nil {
		return StatusResult{}, err
	}
	db, err := s.open(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	defer db.Close()
	if err := checkIntegrity(ctx, db); err != nil {
		return StatusResult{}, err
	}
	meta, err := currentGeneration(ctx, db, selection.Environment, selection.Site)
	if err != nil {
		return StatusResult{}, err
	}
	age := s.now().Sub(meta.generatedAt)
	if age < 0 {
		age = 0
	}
	stale, warnings := staleness(s.now, meta.generatedAt, meta.id)
	return StatusResult{meta.id, meta.environment, meta.site, meta.generatedAt, age, true, stale, meta.source, databaseRelativePath, meta.recordCount, warnings}, nil
}

type generationMeta struct {
	key                           int64
	id, environment, site, source string
	generatedAt                   time.Time
	recordCount                   int
}
type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func currentGeneration(ctx context.Context, q queryRower, environment, site string) (generationMeta, error) {
	var m generationMeta
	var generated string
	err := q.QueryRowContext(ctx, `SELECT g.generation_key,g.id,g.environment,g.site,g.generated_at,g.source,g.record_count FROM current_generations c JOIN generations g ON g.generation_key=c.generation_key WHERE c.environment=? AND c.site=? AND g.complete=1`, environment, site).Scan(&m.key, &m.id, &m.environment, &m.site, &generated, &m.source, &m.recordCount)
	if err != nil {
		return m, fmt.Errorf("read current catalog generation for environment %q and site %q: %w", environment, site, err)
	}
	m.generatedAt, err = time.Parse(time.RFC3339Nano, generated)
	return m, err
}
func searchWhere(key int64, q Query) (string, []any) {
	parts := []string{"WHERE generation_key=?", "requested=1"}
	args := []any{key}
	filters := [][2]string{{"luid", q.LUID}, {"kind", q.Kind}, {"name", q.Name}, {"project_path", q.ProjectPath}, {"owner", q.Owner}}
	for _, f := range filters {
		if f[1] != "" {
			parts = append(parts, f[0]+"=?")
			args = append(args, f[1])
		}
	}
	if q.Text != "" {
		parts = append(parts, `instr(lower(name || ' ' || project_path || ' ' || owner || ' ' || luid),lower(?)) > 0`)
		args = append(args, q.Text)
	}
	return strings.Join(parts, " AND "), args
}
func validateSelection(environment string, selected bool) error {
	if strings.TrimSpace(environment) == "" {
		return errors.New("catalog search requires an environment")
	}
	if !selected {
		return errors.New("catalog search requires a resolved source site")
	}
	return validateField("environment", environment)
}
func normalizedScopes(scopes []string, defaults bool) []string {
	if len(scopes) == 0 && defaults {
		return append([]string(nil), publicScopes...)
	}
	set := map[string]bool{}
	for _, scope := range scopes {
		set[strings.TrimSpace(scope)] = true
	}
	result := make([]string, 0, len(set))
	for scope := range set {
		if scope != "" {
			result = append(result, scope)
		}
	}
	sort.Strings(result)
	return result
}
func validateMetadata(m GenerationMetadata) error {
	if strings.TrimSpace(m.Environment) == "" || m.GeneratedAt.IsZero() {
		return errors.New("catalog generation requires environment and generation time")
	}
	for name, value := range map[string]string{"generation ID": m.ID, "environment": m.Environment, "site": m.Site, "source": m.Source} {
		if err := validateField(name, value); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, scope := range append(append([]string{}, m.RequestedScopes...), m.ImplicitScopes...) {
		if _, ok := batchColumns[scope]; !ok {
			return fmt.Errorf("catalog scope %q is unsupported", scope)
		}
		if seen[scope] {
			return fmt.Errorf("catalog scope %q cannot be both requested and implicit", scope)
		}
		seen[scope] = true
	}
	return nil
}
func validateField(name, value string) error {
	if len(value) > maxFieldBytes {
		return fmt.Errorf("%s exceeds %d-byte limit", name, maxFieldBytes)
	}
	return nil
}
func kindScope(kind string) string {
	switch kind {
	case "user":
		return "users"
	case "group":
		return "groups"
	case "project":
		return "projects"
	case "workbook":
		return "workbooks"
	case "datasource":
		return "datasources"
	case "flow":
		return "flows"
	case "view":
		return "views"
	}
	return ""
}
func staleness(now func() time.Time, generated time.Time, id string) (bool, []string) {
	stale := now().Sub(generated) > staleAfter
	if !stale {
		return false, nil
	}
	return true, []string{fmt.Sprintf("catalog generation %q is older than 12 hours", id)}
}
func queryFingerprint(q Query) string {
	value := struct {
		Text, Kind, Name, ProjectPath, Owner, Environment, Site, LUID string
		Limit                                                         int
	}{strings.ToLower(q.Text), q.Kind, q.Name, q.ProjectPath, q.Owner, q.Environment, q.Site, q.LUID, q.Limit}
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}
func encodeCursor(id, query string, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(id)) + "." + query + "." + strconv.Itoa(offset)
}
func decodeCursor(value string) (string, string, int, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", 0, errors.New("invalid cursor")
	}
	generation, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", 0, err
	}
	query, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(query) != sha256.Size {
		return "", "", 0, errors.New("invalid cursor")
	}
	offset, err := strconv.Atoi(parts[2])
	return string(generation), parts[1], offset, err
}
func digestID(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
