package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jmelahman/kanban/internal/api"
	"github.com/jmelahman/kanban/internal/client"
	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/secrets"
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

func TestInferBoardCreateArgs(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "myrepo")
	mustGit(t, "", "init", "-q", "-b", "main", repo)
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	mustGit(t, repo, "commit", "--allow-empty", "-q", "-m", "init")

	t.Run("infers_repo_and_name_from_cwd", func(t *testing.T) {
		t.Chdir(repo)
		got, err := inferBoardCreateArgs(client.CreateBoardArgs{})
		if err != nil {
			t.Fatal(err)
		}
		if !samePath(got.RepoPath, repo) {
			t.Errorf("RepoPath = %q, want %q", got.RepoPath, repo)
		}
		if got.Name != "myrepo" {
			t.Errorf("Name = %q, want myrepo", got.Name)
		}
	})

	t.Run("infers_from_subdirectory", func(t *testing.T) {
		sub := filepath.Join(repo, "a", "b")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(sub)
		got, err := inferBoardCreateArgs(client.CreateBoardArgs{})
		if err != nil {
			t.Fatal(err)
		}
		if !samePath(got.RepoPath, repo) {
			t.Errorf("RepoPath = %q, want %q", got.RepoPath, repo)
		}
	})

	t.Run("linked_worktree_resolves_to_main_repo", func(t *testing.T) {
		wt := filepath.Join(dir, "wt")
		mustGit(t, repo, "worktree", "add", "-q", wt)
		t.Chdir(wt)
		got, err := inferBoardCreateArgs(client.CreateBoardArgs{})
		if err != nil {
			t.Fatal(err)
		}
		if !samePath(got.RepoPath, repo) {
			t.Errorf("RepoPath = %q, want main repo %q", got.RepoPath, repo)
		}
		if got.Name != "myrepo" {
			t.Errorf("Name = %q, want myrepo", got.Name)
		}
	})

	t.Run("explicit_flags_win", func(t *testing.T) {
		t.Chdir(repo)
		got, err := inferBoardCreateArgs(client.CreateBoardArgs{Name: "Custom", RepoPath: "/elsewhere"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "Custom" || got.RepoPath != "/elsewhere" {
			t.Errorf("explicit args changed: %+v", got)
		}
	})

	t.Run("name_inferred_from_explicit_repo_path", func(t *testing.T) {
		t.Chdir(t.TempDir())
		got, err := inferBoardCreateArgs(client.CreateBoardArgs{RepoPath: "/somewhere/proj"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "proj" {
			t.Errorf("Name = %q, want proj", got.Name)
		}
	})

	t.Run("errors_outside_git_repo", func(t *testing.T) {
		t.Chdir(t.TempDir())
		if _, err := inferBoardCreateArgs(client.CreateBoardArgs{}); err == nil {
			t.Fatal("expected error outside a git repo")
		}
	})

	t.Run("mount_path_only_requires_name", func(t *testing.T) {
		if _, err := inferBoardCreateArgs(client.CreateBoardArgs{MountPath: "/workspace"}); err == nil {
			t.Fatal("expected error for --mount-path without --name")
		}
		got, err := inferBoardCreateArgs(client.CreateBoardArgs{MountPath: "/workspace", Name: "WS"})
		if err != nil {
			t.Fatal(err)
		}
		if got.RepoPath != "" || got.Name != "WS" {
			t.Errorf("mount-path args changed: %+v", got)
		}
	})
}

func TestRunBoardCreateInferred(t *testing.T) {
	srv, store, _ := newKanbanCLITestServer(t)
	repo := filepath.Join(t.TempDir(), "inferred-repo")
	mustGit(t, "", "init", "-q", "-b", "trunk", repo)
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "user.name", "Test")
	mustGit(t, repo, "commit", "--allow-empty", "-q", "-m", "init")
	t.Chdir(repo)

	args, err := inferBoardCreateArgs(client.CreateBoardArgs{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runBoardCreate(t.Context(), srv.URL, &out, args, true); err != nil {
		t.Fatal(err)
	}
	var created db.Board
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &created); err != nil {
		t.Fatalf("output not JSON: %q (%v)", out.String(), err)
	}
	if created.Name != "inferred-repo" {
		t.Errorf("Name = %q, want inferred-repo", created.Name)
	}
	if created.BaseBranch != "trunk" {
		t.Errorf("BaseBranch = %q, want trunk (detected by server)", created.BaseBranch)
	}
	if _, err := store.GetBoard(t.Context(), created.ID); err != nil {
		t.Fatalf("board not in store: %v", err)
	}
}

func TestResolveBoardIdent(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)

	t.Run("explicit_arg_passthrough", func(t *testing.T) {
		got, err := resolveBoardIdent(t.Context(), srv.URL, []string{"some-slug"})
		if err != nil {
			t.Fatal(err)
		}
		if got != "some-slug" {
			t.Errorf("ident = %q, want some-slug", got)
		}
	})

	t.Run("infers_board_from_cwd", func(t *testing.T) {
		t.Chdir(board.RepoPath)
		got, err := resolveBoardIdent(t.Context(), srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if want := strconv.FormatInt(board.ID, 10); got != want {
			t.Errorf("ident = %q, want %q", got, want)
		}

		// The summary of an inferred lookup shows the repo and base branch.
		var out bytes.Buffer
		if err := runBoardGet(t.Context(), srv.URL, &out, got, false); err != nil {
			t.Fatal(err)
		}
		if s := out.String(); !strings.Contains(s, board.Slug) || !strings.Contains(s, "repo=") || !strings.Contains(s, "base=main") {
			t.Errorf("summary missing inferred fields: %q", s)
		}
	})

	t.Run("errors_when_no_board_matches", func(t *testing.T) {
		other := filepath.Join(t.TempDir(), "unrelated")
		mustGit(t, "", "init", "-q", "-b", "main", other)
		t.Chdir(other)
		if _, err := resolveBoardIdent(t.Context(), srv.URL, nil); err == nil {
			t.Fatal("expected error when no board matches the cwd repo")
		}
	})

	t.Run("errors_when_ambiguous", func(t *testing.T) {
		second := &db.Board{
			Name:       "CLI Board 2",
			Slug:       "cli-board-2",
			RepoPath:   board.RepoPath,
			BaseBranch: "main",
		}
		if err := store.CreateBoard(context.Background(), second); err != nil {
			t.Fatal(err)
		}
		t.Chdir(board.RepoPath)
		_, err := resolveBoardIdent(t.Context(), srv.URL, nil)
		if err == nil {
			t.Fatal("expected error when two boards share the repo")
		}
		if !strings.Contains(err.Error(), second.Slug) {
			t.Errorf("ambiguity error should list candidates: %v", err)
		}
	})
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

func TestRunEnvLifecycle(t *testing.T) {
	srv, store, board := newKanbanCLITestServer(t)

	var out bytes.Buffer
	if err := runEnvSet(t.Context(), srv.URL, &out, board.Slug, []string{"MY_API_KEY=s3cret", "OTHER=x"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "s3cret") {
		t.Errorf("env set printed a value: %q", got)
	}

	out.Reset()
	if err := runEnvList(t.Context(), srv.URL, &out, board.Slug); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "KEY") || !strings.Contains(got, "MY_API_KEY") || !strings.Contains(got, "OTHER") {
		t.Errorf("env list missing keys: %q", got)
	}
	if strings.Contains(got, "s3cret") {
		t.Errorf("env list printed a value: %q", got)
	}
	vars, err := store.GetBoardEnvVars(t.Context(), board.ID)
	if err != nil || vars["MY_API_KEY"] != "s3cret" {
		t.Errorf("value not persisted: vars=%v err=%v", vars, err)
	}

	out.Reset()
	if err := runEnvUnset(t.Context(), srv.URL, &out, board.Slug, []string{"MY_API_KEY"}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "MY_API_KEY") {
		t.Errorf("env unset still lists removed key: %q", got)
	}

	// Malformed pair errors before hitting the server.
	if err := runEnvSet(t.Context(), srv.URL, &out, board.Slug, []string{"NO_EQUALS"}); err == nil {
		t.Error("expected error for KEY without =")
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

	envKey, err := secrets.NewRandomKey()
	if err != nil {
		t.Fatal(err)
	}
	box, err := secrets.NewBox(envKey)
	if err != nil {
		t.Fatal(err)
	}
	store.SetEnvCipher(box)

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
