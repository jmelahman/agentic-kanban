package errreport_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/errreport"
)

func newStore(t *testing.T) *db.Store {
	t.Helper()
	s, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// boardSlug mirrors errreport's slugify rule; used in tests to look up the
// auto-created errors board without exposing the package's helper.
func boardSlug(t *testing.T, s *db.Store, name string) *db.Board {
	t.Helper()
	candidate := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	b, err := s.GetBoardBySlug(context.Background(), candidate)
	if err != nil {
		t.Fatalf("GetBoardBySlug(%q): %v", candidate, err)
	}
	return b
}

func TestReporter_DisabledIsNoOp(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: false, BoardName: "Errors"})
	r.Report(context.Background(), "panic", "boom", "stack", nil)

	boards, err := store.ListBoards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 0 {
		t.Fatalf("expected no boards when disabled, got %d", len(boards))
	}
}

func TestReporter_NilIsNoOp(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("nil receiver panicked: %v", rec)
		}
	}()
	var r *errreport.Reporter
	r.Report(context.Background(), "panic", "boom", "stack", nil)
	r.Capture(context.Background(), "http", nil)
	if r.Enabled() {
		t.Fatal("nil reporter should report Enabled() == false")
	}
}

func TestReporter_CreatesFreshTicket(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: true, BoardName: "Errors"})
	r.Report(context.Background(), "panic", "nil pointer dereference", "main.go:42\nfoo.go:10", map[string]string{"path": "/api/x"})

	board := boardSlug(t, store, "errors")
	cols, err := store.ListColumns(context.Background(), board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != 3 || cols[0].Name != "New" || cols[1].Name != "Investigating" || cols[2].Name != "Resolved" {
		t.Fatalf("unexpected columns: %+v", cols)
	}
	tickets, err := store.ListTickets(context.Background(), board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(tickets))
	}
	if tickets[0].ColumnID != cols[0].ID {
		t.Fatalf("ticket should land in 'New' column %d, got %d", cols[0].ID, tickets[0].ColumnID)
	}
	if tickets[0].Title != "nil pointer dereference" {
		t.Fatalf("title = %q", tickets[0].Title)
	}
	// Title body should reference the meta we passed.
	if !strings.Contains(tickets[0].Body, "/api/x") {
		t.Fatalf("body missing meta path: %q", tickets[0].Body)
	}
}

func TestReporter_DeduplicatesOnSecondOccurrence(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: true, BoardName: "Errors"})
	stack := "main.go:42\nfoo.go:10\nbar.go:99"
	r.Report(context.Background(), "panic", "boom", stack, nil)
	r.Report(context.Background(), "panic", "boom", stack, nil)

	board := boardSlug(t, store, "errors")
	tickets, err := store.ListTickets(context.Background(), board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 1 {
		t.Fatalf("expected dedup to keep ticket count at 1, got %d", len(tickets))
	}
	if !strings.Contains(tickets[0].Body, "Seen again at") {
		t.Fatalf("body should be updated with 'Seen again': %q", tickets[0].Body)
	}
}

func TestReporter_IgnoresArchivedTicket(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: true, BoardName: "Errors"})
	stack := "main.go:42\nfoo.go:10"
	r.Report(context.Background(), "panic", "boom", stack, nil)

	board := boardSlug(t, store, "errors")
	tickets, _ := store.ListTickets(context.Background(), board.ID)
	if len(tickets) != 1 {
		t.Fatalf("setup: expected 1 ticket, got %d", len(tickets))
	}
	if err := store.ArchiveTicket(context.Background(), tickets[0].ID); err != nil {
		t.Fatal(err)
	}

	r.Report(context.Background(), "panic", "boom", stack, nil)

	open, err := store.ListTickets(context.Background(), board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 {
		t.Fatalf("after archive+report, expected 1 open ticket, got %d", len(open))
	}
	if open[0].ID == tickets[0].ID {
		t.Fatalf("expected a fresh ticket id; archived dedup leak")
	}
}

func TestReporter_RecursionGuard(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: true, BoardName: "Errors"})
	// Close the store so all DB writes fail. Report must not panic, must not
	// recurse via its own log, and must return.
	_ = store.Close()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("Report panicked on closed DB: %v", rec)
		}
	}()
	r.Report(context.Background(), "panic", "boom", "stack", nil)
}

func TestReporter_ConfigBoardNameDefault(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: true, BoardName: ""})
	r.Report(context.Background(), "panic", "boom", "stack", nil)
	// Default board name "Errors" → slug "errors".
	if _, err := store.GetBoardBySlug(context.Background(), "errors"); err != nil {
		t.Fatalf("expected default board name 'Errors': %v", err)
	}
}

func TestReporter_CustomBoardName(t *testing.T) {
	store := newStore(t)
	r := errreport.New(store, errreport.Config{Enabled: true, BoardName: "Bug Tracker"})
	r.Report(context.Background(), "panic", "boom", "stack", nil)
	if _, err := store.GetBoardBySlug(context.Background(), "bug-tracker"); err != nil {
		t.Fatalf("expected custom board name: %v", err)
	}
}
