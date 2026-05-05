package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jmelahman/kanban/internal/api"
	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/errreport"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/session"
)

// reportEnv stands up a minimal HTTP server with a real reporter wired in.
// Skips the git/repo seeding that newEnv does — these tests don't touch
// boards or sessions, just the /api/errors endpoint.
type reportEnv struct {
	srv      *httptest.Server
	store    *db.Store
	reporter *errreport.Reporter
}

func newReportEnv(t *testing.T, enabled bool) *reportEnv {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "kanban.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	dockerCli, err := docker.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dockerCli.Close() })
	hookRunner := hooks.NewRunner(store)
	sessionMgr := session.NewManager(store, dockerCli, hookRunner)

	reporter := errreport.New(store, errreport.Config{Enabled: enabled, BoardName: "Errors"})
	cfg := &config.Config{DataDir: dir, PortRangeStart: 13000, PortRangeEnd: 13099}

	handler := api.NewMux(api.Deps{
		Store: store, Docker: dockerCli, Sessions: sessionMgr, Hooks: hookRunner, Config: cfg, Reporter: reporter,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &reportEnv{srv: srv, store: store, reporter: reporter}
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestPostErrors_DisabledReporter_NoTicket(t *testing.T) {
	env := newReportEnv(t, false)
	resp := postJSON(t, env.srv.URL+"/api/errors", map[string]string{
		"message": "boom", "stack": "x.ts:1", "source": "boundary",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body)

	boards, err := env.store.ListBoards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(boards) != 0 {
		t.Fatalf("disabled reporter should not create board; got %d", len(boards))
	}
}

func TestPostErrors_EnabledReporter_FilesTicket(t *testing.T) {
	env := newReportEnv(t, true)
	resp := postJSON(t, env.srv.URL+"/api/errors", map[string]string{
		"message": "TypeError: x is undefined",
		"stack":   "at App.tsx:1\n at index.tsx:5",
		"source":  "boundary",
		"url":     "http://localhost:5173/",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d; want 204", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body)

	board, err := env.store.GetBoardBySlug(context.Background(), "errors")
	if err != nil {
		t.Fatalf("expected errors board: %v", err)
	}
	tickets, err := env.store.ListTickets(context.Background(), board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(tickets))
	}
	if tickets[0].Title != "TypeError: x is undefined" {
		t.Fatalf("title = %q", tickets[0].Title)
	}
}

func TestPostErrors_DedupOnSecondPost(t *testing.T) {
	env := newReportEnv(t, true)
	body := map[string]string{
		"message": "boom",
		"stack":   "a.ts:1\n b.ts:2",
		"source":  "boundary",
	}
	for i := 0; i < 2; i++ {
		resp := postJSON(t, env.srv.URL+"/api/errors", body)
		resp.Body.Close()
	}
	board, err := env.store.GetBoardBySlug(context.Background(), "errors")
	if err != nil {
		t.Fatalf("expected errors board: %v", err)
	}
	tickets, _ := env.store.ListTickets(context.Background(), board.ID)
	if len(tickets) != 1 {
		t.Fatalf("dedup failed: got %d tickets", len(tickets))
	}
}
