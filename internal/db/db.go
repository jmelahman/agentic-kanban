package db

import (
	_ "embed"
	"fmt"

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

// Open opens a Store backed by a SQLite database at path and applies the
// embedded schema. The sentinel path ":memory:" opens a process-local
// in-memory database (shared cache so the connection pool sees one DB) and
// skips on-disk file setup; data is discarded when Close is called.
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
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }
