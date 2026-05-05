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
		dsn = fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
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
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// migrate applies idempotent ALTER TABLE statements for columns added after
// the original schema. SQLite's CREATE TABLE IF NOT EXISTS won't pick up new
// columns on existing databases, so we ADD COLUMN here and ignore the
// "duplicate column name" error that fires once the column is in place.
func migrate(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE sessions ADD COLUMN pr_number INTEGER`,
		`ALTER TABLE sessions ADD COLUMN pr_url TEXT`,
		`ALTER TABLE boards ADD COLUMN branch_prefix TEXT`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }
