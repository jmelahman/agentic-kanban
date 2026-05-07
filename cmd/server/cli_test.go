package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmelahman/kanban/internal/api"
	"github.com/jmelahman/kanban/internal/client"
	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/session"
)

func TestRunBoardList(t *testing.T) {
	srv, _, board := newKanbanCLITestServer(t)
	var out bytes.Buffer
	if err := runBoardList(t.Context(), srv.URL, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "SLUG") || !strings.Contains(got, "NAME") {
		t.Errorf("missing header: %q", got)
	}
	if !strings.Contains(got, board.Slug) || !strings.Contains(got, board.Name) {
		t.Errorf("board not in output: %q", got)
	}
}

func TestRunTicketCreate(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)

	t.Run("default_column", func(t *testing.T) {
		var out bytes.Buffer
		err := runTicketCreate(t.Context(), srv.URL, &out, client.CreateTicketArgs{
			Board: board.Slug,
			Title: "Via CLI",
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Via CLI") {
			t.Errorf("summary missing title: %q", out.String())
		}
		cols, _ := store.ListColumns(t.Context(), board.ID)
		tickets, _ := store.ListTickets(t.Context(), board.ID)
		var found *db.Ticket
		for i := range tickets {
			if tickets[i].Title == "Via CLI" {
				found = &tickets[i]
			}
		}
		if found == nil {
			t.Fatalf("ticket not created; got %+v", tickets)
		}
		if found.ColumnID != cols[0].ID {
			t.Errorf("default column = %d, want leftmost %d", found.ColumnID, cols[0].ID)
		}
	})

	t.Run("named_column_json_output", func(t *testing.T) {
		var out bytes.Buffer
		err := runTicketCreate(t.Context(), srv.URL, &out, client.CreateTicketArgs{
			Board:  board.Slug,
			Title:  "Named Col",
			Column: "in progress",
		}, true)
		if err != nil {
			t.Fatal(err)
		}
		var tk db.Ticket
		if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &tk); err != nil {
			t.Fatalf("output not JSON: %q (%v)", out.String(), err)
		}
		cols, _ := store.ListColumns(t.Context(), board.ID)
		if tk.ColumnID != cols[1].ID {
			t.Errorf("ColumnID = %d, want %d (In Progress)", tk.ColumnID, cols[1].ID)
		}
	})

	t.Run("server_error_propagates", func(t *testing.T) {
		var out bytes.Buffer
		err := runTicketCreate(t.Context(), srv.URL, &out, client.CreateTicketArgs{
			Board: "no-such-board",
			Title: "x",
		}, false)
		if err == nil {
			t.Fatal("expected error for unknown board")
		}
	})
}

func TestRunBoardCreateAndDelete(t *testing.T) {
	srv, store, _ := newKanbanCLITestServer(t)
	repoPath := filepath.Join(t.TempDir(), "repo2")
	mustGit(t, "", "init", "-q", "-b", "main", repoPath)
	mustGit(t, repoPath, "config", "user.email", "test@example.com")
	mustGit(t, repoPath, "config", "user.name", "Test")
	mustGit(t, repoPath, "commit", "--allow-empty", "-q", "-m", "init")

	var out bytes.Buffer
	err := runBoardCreate(t.Context(), srv.URL, &out, client.CreateBoardArgs{
		Name:       "Second Board",
		RepoPath:   repoPath,
		BaseBranch: "main",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	var created db.Board
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &created); err != nil {
		t.Fatalf("output not JSON: %q (%v)", out.String(), err)
	}
	if created.ID == 0 || created.Name != "Second Board" {
		t.Fatalf("board not created in response: %+v", created)
	}
	got, err := store.GetBoard(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("board not in store: %v", err)
	}
	if got.Slug != created.Slug {
		t.Errorf("slug mismatch %q vs %q", got.Slug, created.Slug)
	}

	out.Reset()
	if err := runBoardDelete(t.Context(), srv.URL, &out, created.Slug); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetBoard(t.Context(), created.ID); err == nil {
		t.Error("board still exists after delete")
	}
}

func TestRunBoardStateAndArchived(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)
	tk := &db.Ticket{BoardID: board.ID, Title: "to archive"}
	cols, _ := store.ListColumns(t.Context(), board.ID)
	tk.ColumnID = cols[0].ID
	tk.Slug = "to-archive"
	if err := store.CreateTicket(t.Context(), tk); err != nil {
		t.Fatal(err)
	}
	if err := store.ArchiveTicket(t.Context(), tk.ID); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runBoardArchived(t.Context(), srv.URL, &out, board.Slug); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "to archive") {
		t.Errorf("archived listing missing ticket: %q", out.String())
	}

	out.Reset()
	if err := runBoardState(t.Context(), srv.URL, &out, board.Slug); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"columns"`) || !strings.Contains(out.String(), `"tickets"`) {
		t.Errorf("state output missing keys: %q", out.String())
	}

	out.Reset()
	if err := runBoardArchivedClear(t.Context(), srv.URL, &out, board.Slug); err != nil {
		t.Fatal(err)
	}
	remaining, _ := store.ListArchivedTickets(t.Context(), board.ID)
	if len(remaining) != 0 {
		t.Errorf("expected 0 archived after clear, got %d", len(remaining))
	}
}

func TestRunTicketLifecycle(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)
	cols, _ := store.ListColumns(t.Context(), board.ID)
	tk := &db.Ticket{BoardID: board.ID, ColumnID: cols[0].ID, Title: "lifecycle", Slug: "lifecycle"}
	if err := store.CreateTicket(t.Context(), tk); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	newTitle := "renamed"
	if err := runTicketUpdate(t.Context(), srv.URL, &out, tk.ID,
		client.UpdateTicketArgs{Title: &newTitle}, false); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetTicket(t.Context(), tk.ID)
	if got.Title != newTitle {
		t.Errorf("title not updated: %q", got.Title)
	}

	out.Reset()
	if err := runTicketMove(t.Context(), srv.URL, &out, tk.ID,
		client.MoveTicketArgs{ColumnID: cols[1].ID, Position: 0}); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetTicket(t.Context(), tk.ID)
	if got.ColumnID != cols[1].ID {
		t.Errorf("column not changed: got %d want %d", got.ColumnID, cols[1].ID)
	}

	out.Reset()
	c := client.New(srv.URL, nil)
	if err := c.ArchiveTicket(t.Context(), tk.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetTicket(t.Context(), tk.ID)
	if got.ArchivedAt == nil {
		t.Errorf("ticket not archived")
	}

	if err := c.UnarchiveTicket(t.Context(), tk.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetTicket(t.Context(), tk.ID)
	if got.ArchivedAt != nil {
		t.Errorf("ticket still archived")
	}
}

func TestRunColumnArchiveAll(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)
	cols, _ := store.ListColumns(t.Context(), board.ID)
	for i := 0; i < 3; i++ {
		tk := &db.Ticket{BoardID: board.ID, ColumnID: cols[0].ID, Title: "t", Slug: "t"}
		if err := store.CreateTicket(t.Context(), tk); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := runColumnArchiveAll(t.Context(), srv.URL, &out, cols[0].ID); err != nil {
		t.Fatal(err)
	}
	tickets, _ := store.ListTicketsInColumn(t.Context(), cols[0].ID)
	if len(tickets) != 0 {
		t.Errorf("expected column empty after archive-all, got %d tickets", len(tickets))
	}
	archived, _ := store.ListArchivedTickets(t.Context(), board.ID)
	if len(archived) != 3 {
		t.Errorf("expected 3 archived tickets, got %d", len(archived))
	}
}

func newKanbanCLITestServer(t *testing.T) (*httptest.Server, *db.Store, *db.Board) {
	t.Helper()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	mustGit(t, "", "init", "-q", "-b", "main", repoPath)
	mustGit(t, repoPath, "config", "user.email", "test@example.com")
	mustGit(t, repoPath, "config", "user.name", "Test")
	mustGit(t, repoPath, "commit", "--allow-empty", "-q", "-m", "init")

	store, err := db.Open(filepath.Join(dir, "kanban.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	dockerCli, err := docker.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dockerCli.Close() })

	cfg := &config.Config{DataDir: dir, PortRangeStart: 13000, PortRangeEnd: 13099}
	hookRunner := hooks.NewRunner(store)
	sessionMgr := session.NewManager(store, dockerCli, hookRunner)
	handler := api.NewMux(api.Deps{
		Store: store, Docker: dockerCli, Sessions: sessionMgr, Hooks: hookRunner, Config: cfg,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	board := &db.Board{
		Name:         "CLI Board",
		Slug:         "cli-board",
		RepoPath:     repoPath,
		WorktreeRoot: filepath.Join(dir, "worktrees", "cli"),
		BaseBranch:   "main",
	}
	if err := store.CreateBoard(context.Background(), board); err != nil {
		t.Fatal(err)
	}
	return srv, store, board
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
