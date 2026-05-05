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

func TestRunListBoards(t *testing.T) {
	srv, _, board := newKanbanCLITestServer(t)
	var out bytes.Buffer
	if err := runListBoards(t.Context(), srv.URL, &out); err != nil {
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

func TestRunCreateTicket(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)

	t.Run("default_column", func(t *testing.T) {
		var out bytes.Buffer
		err := runCreateTicket(t.Context(), srv.URL, &out, client.CreateTicketArgs{
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
		err := runCreateTicket(t.Context(), srv.URL, &out, client.CreateTicketArgs{
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
		err := runCreateTicket(t.Context(), srv.URL, &out, client.CreateTicketArgs{
			Board: "no-such-board",
			Title: "x",
		}, false)
		if err == nil {
			t.Fatal("expected error for unknown board")
		}
	})
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
