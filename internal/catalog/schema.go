package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var schemaStatements = []string{
	`CREATE TABLE catalog_schema (singleton INTEGER PRIMARY KEY CHECK(singleton=1), version INTEGER NOT NULL, signature TEXT NOT NULL) STRICT`,
	`INSERT INTO catalog_schema(singleton,version,signature) VALUES(1,1,'tadx-catalog-v1')`,
	`CREATE TABLE generations (generation_key INTEGER PRIMARY KEY,id TEXT,fingerprint TEXT,environment TEXT NOT NULL,site TEXT NOT NULL,generated_at TEXT NOT NULL,complete INTEGER NOT NULL CHECK(complete IN (0,1)),source TEXT NOT NULL,record_count INTEGER NOT NULL CHECK(record_count>=0),created_at TEXT NOT NULL,UNIQUE(environment,site,id),UNIQUE(environment,site,fingerprint),CHECK((complete=0 AND id IS NULL AND fingerprint IS NULL) OR (complete=1 AND id IS NOT NULL AND fingerprint IS NOT NULL))) STRICT`,
	`CREATE TABLE current_generations (environment TEXT NOT NULL,site TEXT NOT NULL,generation_key INTEGER NOT NULL UNIQUE REFERENCES generations(generation_key) ON DELETE RESTRICT,PRIMARY KEY(environment,site)) STRICT`,
	`CREATE TABLE generation_scopes (generation_key INTEGER NOT NULL REFERENCES generations(generation_key) ON DELETE CASCADE,scope TEXT NOT NULL CHECK(scope IN ('users','groups','projects','workbooks','datasources','flows','views','permissions')),requested INTEGER NOT NULL CHECK(requested IN (0,1)),complete INTEGER NOT NULL CHECK(complete IN (0,1)),PRIMARY KEY(generation_key,scope)) STRICT`,
	resourceDDL("users", `id TEXT NOT NULL,name TEXT NOT NULL,email TEXT NOT NULL,site_role TEXT NOT NULL,last_login TEXT NOT NULL`, "id"),
	resourceDDL("groups", `id TEXT NOT NULL,name TEXT NOT NULL,domain TEXT NOT NULL`, "id"),
	resourceDDL("projects", `id TEXT NOT NULL,name TEXT NOT NULL,parent_project_id TEXT NOT NULL,description TEXT NOT NULL,owner_id TEXT NOT NULL`, "id"),
	resourceDDL("workbooks", `id TEXT NOT NULL,name TEXT NOT NULL,project_id TEXT NOT NULL,owner_id TEXT NOT NULL,size INTEGER,updated_at TEXT NOT NULL`, "id"),
	resourceDDL("datasources", `id TEXT NOT NULL,name TEXT NOT NULL,project_id TEXT NOT NULL,owner_id TEXT NOT NULL,updated_at TEXT NOT NULL`, "id"),
	resourceDDL("flows", `id TEXT NOT NULL,name TEXT NOT NULL,project_id TEXT NOT NULL,owner_id TEXT NOT NULL,updated_at TEXT NOT NULL`, "id"),
	resourceDDL("views", `id TEXT NOT NULL,name TEXT NOT NULL,workbook_id TEXT NOT NULL`, "id"),
	`CREATE TABLE permissions (generation_key INTEGER NOT NULL REFERENCES generations(generation_key) ON DELETE CASCADE,content_type TEXT NOT NULL,content_id TEXT NOT NULL,grantee_type TEXT NOT NULL,grantee_id TEXT NOT NULL,capability TEXT NOT NULL,mode TEXT NOT NULL,PRIMARY KEY(generation_key,content_type,content_id,grantee_type,grantee_id,capability)) STRICT`,
	`CREATE TABLE catalog_records (generation_key INTEGER NOT NULL REFERENCES generations(generation_key) ON DELETE CASCADE,luid TEXT NOT NULL,kind TEXT NOT NULL,name TEXT NOT NULL,project_path TEXT NOT NULL,owner TEXT NOT NULL,requested INTEGER NOT NULL CHECK(requested IN (0,1)),PRIMARY KEY(generation_key,kind,luid)) STRICT`,
	`CREATE INDEX generations_source_idx ON generations(environment,site,complete,generated_at DESC)`,
	`CREATE INDEX generation_scopes_requested_idx ON generation_scopes(generation_key,requested,scope)`,
	`CREATE INDEX projects_parent_idx ON projects(generation_key,parent_project_id)`,
	`CREATE INDEX projects_owner_idx ON projects(generation_key,owner_id)`,
	`CREATE INDEX workbooks_project_idx ON workbooks(generation_key,project_id)`, `CREATE INDEX workbooks_owner_idx ON workbooks(generation_key,owner_id)`,
	`CREATE INDEX datasources_project_idx ON datasources(generation_key,project_id)`, `CREATE INDEX flows_project_idx ON flows(generation_key,project_id)`, `CREATE INDEX views_workbook_idx ON views(generation_key,workbook_id)`,
	`CREATE INDEX permissions_content_idx ON permissions(generation_key,content_type,content_id)`, `CREATE INDEX permissions_grantee_idx ON permissions(generation_key,grantee_type,grantee_id)`,
	`CREATE INDEX catalog_records_order_idx ON catalog_records(generation_key,requested,kind,name,project_path,luid)`, `CREATE INDEX catalog_records_lookup_idx ON catalog_records(generation_key,requested,kind,name,project_path)`,
}

var requiredTables = []string{"catalog_schema", "generations", "current_generations", "generation_scopes", "users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions", "catalog_records"}

func resourceDDL(table, columns, key string) string {
	return fmt.Sprintf(`CREATE TABLE %s (generation_key INTEGER NOT NULL REFERENCES generations(generation_key) ON DELETE CASCADE,%s,PRIMARY KEY(generation_key,%s)) STRICT`, table, columns, key)
}
func initializeSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		return fmt.Errorf("enable catalog WAL: %w", err)
	}
	if _, err := db.ExecContext(ctx, `PRAGMA synchronous=FULL`); err != nil {
		return fmt.Errorf("configure catalog synchronization: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, statement := range schemaStatements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize catalog schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA user_version=1`); err != nil {
		return fmt.Errorf("version catalog schema: %w", err)
	}
	return checkIntegrity(ctx, db)
}
func validateSchema(ctx context.Context, db *sql.DB) error {
	var userVersion int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&userVersion); err != nil {
		return fmt.Errorf("read catalog schema version: %w", err)
	}
	if userVersion != schemaVersion {
		return fmt.Errorf("catalog schema version %d is unsupported", userVersion)
	}
	var version int
	var signature string
	if err := db.QueryRowContext(ctx, `SELECT version,signature FROM catalog_schema WHERE singleton=1`).Scan(&version, &signature); err != nil {
		return fmt.Errorf("catalog schema is missing or unreadable: %w", err)
	}
	if version != schemaVersion || signature != "tadx-catalog-v1" {
		return fmt.Errorf("catalog schema version %d is unsupported", version)
	}
	for _, table := range requiredTables {
		var name string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_schema WHERE type='table' AND name=?`, table).Scan(&name)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("catalog schema is corrupt: table %q is missing", table)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func checkIntegrity(ctx context.Context, q queryRower) error {
	var integrity string
	if err := q.QueryRowContext(ctx, `PRAGMA quick_check(1)`).Scan(&integrity); err != nil {
		return fmt.Errorf("check catalog integrity: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("catalog integrity check failed: %s", integrity)
	}
	return nil
}
