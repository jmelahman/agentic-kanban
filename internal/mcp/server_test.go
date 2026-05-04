package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmelahman/kanban/internal/api"
	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/session"
)

// TestRun_CreateTicketFlow drives the MCP server end-to-end: real kanban
// HTTP stack via httptest.Server, MCP server speaking JSON-RPC over an
// in-memory pipe, and asserts the ticket lands in the SQLite store.
func TestRun_CreateTicketFlow(t *testing.T) {
	srv, store, board := newKanbanTestServer(t)

	in, stdin := io.Pipe()
	stdout, out := io.Pipe()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- run(ctx, srv.URL, in, out, srv.Client()) }()

	send := func(payload string) {
		if _, err := stdin.Write([]byte(payload + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	reader := bufio.NewReader(stdout)
	read := func() map[string]json.RawMessage {
		t.Helper()
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var resp map[string]json.RawMessage
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		if e, ok := resp["error"]; ok {
			t.Fatalf("rpc error: %s", string(e))
		}
		return resp
	}

	// 1. initialize
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	initResp := read()
	if !strings.Contains(string(initResp["result"]), `"serverInfo"`) {
		t.Fatalf("initialize missing serverInfo: %s", initResp["result"])
	}
	send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	// 2. tools/list
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	listResp := read()
	if !strings.Contains(string(listResp["result"]), `"create_ticket"`) ||
		!strings.Contains(string(listResp["result"]), `"list_boards"`) {
		t.Fatalf("tools/list missing tools: %s", listResp["result"])
	}

	// 3. tools/call create_ticket by slug, no column
	callJSON, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{
			"name": "create_ticket",
			"arguments": map[string]any{
				"board": board.Slug,
				"title": "From MCP",
				"body":  "via mcp test",
			},
		},
	})
	send(string(callJSON))
	callResp := read()
	if strings.Contains(string(callResp["result"]), `"isError":true`) {
		t.Fatalf("tools/call returned error: %s", callResp["result"])
	}

	// Verify the ticket landed in the store.
	tickets, err := store.ListTickets(ctx, board.ID)
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	var found *db.Ticket
	for i := range tickets {
		if tickets[i].Title == "From MCP" {
			found = &tickets[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("ticket not created; got %+v", tickets)
	}
	cols, _ := store.ListColumns(ctx, board.ID)
	if found.ColumnID != cols[0].ID {
		t.Errorf("ticket landed in column %d, want leftmost %d", found.ColumnID, cols[0].ID)
	}

	// 4. unknown method -> JSON-RPC -32601
	send(`{"jsonrpc":"2.0","id":4,"method":"nope"}`)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(line), `"code":-32601`) {
		t.Errorf("expected method-not-found, got %s", line)
	}

	// Closing stdin makes Run return.
	stdin.Close()
	out.Close()
	if err := <-done; err != nil {
		t.Errorf("run returned error: %v", err)
	}
}

func newKanbanTestServer(t *testing.T) (*httptest.Server, *db.Store, *db.Board) {
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
		Name:           "MCP Board",
		Slug:           "mcp-board",
		SourceRepoPath: repoPath,
		WorktreeRoot:   filepath.Join(dir, "worktrees", "mcp"),
		BaseBranch:     "main",
	}
	if err := store.CreateBoard(t.Context(), board); err != nil {
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
