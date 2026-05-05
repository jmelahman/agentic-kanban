package db_test

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/jmelahman/kanban/internal/db"
)

// expectedTables is the set of tables schema.sql is supposed to create on a
// fresh open. If schema.sql gains a table, this list goes with it; the test
// then guards against accidental schema drift.
var expectedTables = []string{
	"boards",
	"columns",
	"hook_configs",
	"port_allocations",
	"sessions",
	"task_runs",
	"tickets",
}

func TestOpen_InMemory(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	assertSchema(t, store)
	assertBoardLifecycle(t, store)
}

func TestOpen_FreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kanban.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open(%q): %v", path, err)
	}
	t.Cleanup(func() { _ = store.Close() })

	assertSchema(t, store)
	assertBoardLifecycle(t, store)
}

// TestOpen_Reopen exercises the migrate path against an already-populated
// schema, which is what production hits on every startup after the first one.
func TestOpen_Reopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kanban.db")
	first, err := db.Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	second, err := db.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })
	assertSchema(t, second)
}

func assertSchema(t *testing.T, store *db.Store) {
	t.Helper()
	rows, err := store.DB().Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("query tables: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(got)
	if len(got) != len(expectedTables) {
		t.Fatalf("tables = %v; want %v", got, expectedTables)
	}
	for i, name := range expectedTables {
		if got[i] != name {
			t.Errorf("tables[%d] = %q; want %q (full: %v)", i, got[i], name, got)
		}
	}
}

func assertBoardLifecycle(t *testing.T, store *db.Store) {
	t.Helper()
	ctx := t.Context()

	b := &db.Board{Name: "Smoke", Slug: "smoke", BaseBranch: "main", RepoPath: "/tmp/x"}
	if err := store.CreateBoard(ctx, b); err != nil {
		t.Fatalf("CreateBoard: %v", err)
	}
	if b.ID == 0 {
		t.Fatal("CreateBoard did not assign id")
	}

	cols, err := store.ListColumns(ctx, b.ID)
	if err != nil {
		t.Fatalf("ListColumns: %v", err)
	}
	wantCols := []string{"Backlog", "In Progress", "Review", "Done"}
	if len(cols) != len(wantCols) {
		t.Fatalf("default columns = %d; want %d (%v)", len(cols), len(wantCols), wantCols)
	}
	for i, c := range cols {
		if c.Name != wantCols[i] || c.Position != i {
			t.Errorf("col[%d] = {%q, pos %d}; want {%q, pos %d}", i, c.Name, c.Position, wantCols[i], i)
		}
	}

	tk := &db.Ticket{BoardID: b.ID, ColumnID: cols[0].ID, Title: "first", Slug: "first"}
	if err := store.CreateTicket(ctx, tk); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if tk.ID == 0 || tk.Position != 1 {
		t.Errorf("ticket = %+v; want id != 0 and position 1", tk)
	}

	got, err := store.ListTickets(ctx, b.ID)
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	if len(got) != 1 || got[0].ID != tk.ID {
		t.Errorf("ListTickets = %+v; want one row with id %d", got, tk.ID)
	}
}
