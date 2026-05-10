package db

import (
	_ "embed"
	"fmt"
	"strings"

	"database/sql"

	_ "modernc.org/sqlite"

	"github.com/jmelahman/kanban/internal/config"
)

//go:embed schema.sql
var schemaSQL string

// pragmas applied to on-disk databases. WAL + synchronous=NORMAL is the
// recommended pairing for local SQLite apps: durable on crash, ~3× fewer
// fsyncs than FULL. temp_store=MEMORY avoids disk spills for the position-
// shifting transactions in MoveTicket. cache_size is in KiB when negative
// (~20 MB here); mmap_size is in bytes (128 MB).
const onDiskPragmas = "_pragma=journal_mode(WAL)" +
	"&_pragma=foreign_keys(1)" +
	"&_pragma=busy_timeout(5000)" +
	"&_pragma=synchronous(NORMAL)" +
	"&_pragma=temp_store(MEMORY)" +
	"&_pragma=cache_size(-20000)" +
	"&_pragma=mmap_size(134217728)"

type Store struct {
	db *sql.DB
}

// Open opens a Store backed by a SQLite database at path, applies the embedded
// schema, and runs idempotent migrations. The sentinel path ":memory:" opens
// a process-local in-memory database (shared cache so the connection pool sees
// one DB) and skips on-disk file setup; data is discarded when Close is called.
func Open(path string) (*Store, error) {
	var dsn string
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	} else {
		if err := config.MakeFileAll(path); err != nil {
			return nil, fmt.Errorf("ensure db file: %w", err)
		}
		dsn = fmt.Sprintf("file:%s?%s", path, onDiskPragmas)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if path == ":memory:" {
		// Pin to a single connection so the shared in-memory DB isn't dropped
		// when the pool churns; modernc/sqlite tears the DB down once the last
		// connection closes.
		db.SetMaxOpenConns(1)
	} else {
		// Cap the pool to keep connection churn (and "database is locked"
		// retries under busy_timeout) bounded. WAL mode supports concurrent
		// readers + one writer, so 8 is plenty for a local desktop app.
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(4)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := requireCompatibleSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// requireCompatibleSchema rejects pre-release databases created before
// sessions.pr_state was made nullable. CREATE TABLE IF NOT EXISTS leaves an
// existing table's column definitions alone, so an old DB silently runs with
// the wrong nullability — surface that loudly instead of letting it through.
func requireCompatibleSchema(db *sql.DB) error {
	var stale int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='pr_state' AND "notnull"=1`,
	).Scan(&stale)
	if err != nil {
		return fmt.Errorf("inspect sessions schema: %w", err)
	}
	if stale > 0 {
		return fmt.Errorf("database has the pre-release schema (sessions.pr_state NOT NULL); delete the DB file and restart to pick up the breaking schema change")
	}
	return nil
}

// migrate applies idempotent ALTER TABLE statements for columns added after
// the original schema. SQLite's CREATE TABLE IF NOT EXISTS won't pick up new
// columns on existing databases, so we ADD COLUMN here and ignore the
// "duplicate column name" error that fires once the column is in place.
func migrate(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE sessions ADD COLUMN pr_number INTEGER`,
		`ALTER TABLE sessions ADD COLUMN pr_url TEXT`,
		`ALTER TABLE sessions ADD COLUMN pr_title TEXT`,
		`ALTER TABLE boards ADD COLUMN branch_prefix TEXT`,
		`ALTER TABLE boards ADD COLUMN git_author_name TEXT`,
		`ALTER TABLE boards ADD COLUMN git_author_email TEXT`,
		`ALTER TABLE tickets ADD COLUMN fingerprint TEXT`,
		`ALTER TABLE sessions ADD COLUMN claude_session_id TEXT`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tickets_fingerprint
		ON tickets(board_id, fingerprint)
		WHERE fingerprint IS NOT NULL AND archived_at IS NULL`); err != nil {
		return fmt.Errorf("create idx_tickets_fingerprint: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }
