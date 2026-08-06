package api_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmelahman/local-preview/orchestrator"

	"github.com/jmelahman/kanban/internal/db"
)

// previewManifest is a minimal preview.toml whose builds need only a shell.
const previewManifest = `
[frontend]
path  = "web"
build = [["sh", "-c", "mkdir -p dist && cp index.html dist/"]]
dist  = "dist"

[backend]
path        = "srv"
build       = [["true"]]
run         = ["./never-started"]
health_path = "/api/health"
`

// seedPreviewableRepo makes the env's board repo deployable: preview.toml
// plus trivial frontend/backend files, committed on main.
func seedPreviewableRepo(t *testing.T, e *testEnv) {
	t.Helper()
	files := map[string]string{
		"preview.toml":   previewManifest,
		"web/index.html": "<html>ticket preview</html>",
		"srv/main.txt":   "backend-ish",
	}
	for name, content := range files {
		p := filepath.Join(e.repoPath, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustGit(t, e.repoPath, "add", "-A")
	mustGit(t, e.repoPath, "commit", "-qm", "add preview manifest")
}

func TestSessionPreviews(t *testing.T) {
	e := newEnv(t)
	seedPreviewableRepo(t, e)
	board := e.seedBoard("Demo Board")
	ticket := e.seedTicket(board, "Add feature")
	sess := e.seedSession(ticket)

	// The session branch exists in the board repo (normally created by the
	// worktree machinery).
	mustGit(t, e.repoPath, "branch", sess.BranchName)

	// No deploys yet — but the endpoint works before anything is registered.
	resp := e.get(fmt.Sprintf("/api/sessions/%d/previews", sess.ID))
	assertStatus(t, resp, 200)
	if got := decodeJSON[[]orchestrator.Deploy](t, resp); len(got) != 0 {
		t.Fatalf("expected no deploys, got %+v", got)
	}

	// Deploy the branch tip.
	resp = e.post(fmt.Sprintf("/api/sessions/%d/previews", sess.ID), nil)
	assertStatus(t, resp, 202)
	deploy := decodeJSON[orchestrator.Deploy](t, resp)
	if deploy.Ref != sess.BranchName || deploy.Repo != "demo-board" {
		t.Fatalf("unexpected deploy: %+v", deploy)
	}

	// Poll the session-scoped list until the deploy is ready.
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp = e.get(fmt.Sprintf("/api/sessions/%d/previews", sess.ID))
		assertStatus(t, resp, 200)
		deploys := decodeJSON[[]orchestrator.Deploy](t, resp)
		if len(deploys) == 1 && deploys[0].Status == orchestrator.StatusReady {
			if deploys[0].PreviewURL == "" {
				t.Fatalf("ready deploy missing preview_url: %+v", deploys[0])
			}
			break
		}
		if len(deploys) == 1 && deploys[0].Status == orchestrator.StatusFailed {
			t.Fatalf("deploy failed: %s", deploys[0].Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("deploy never became ready: %+v", deploys)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Build logs are exposed.
	resp = e.get(fmt.Sprintf("/api/previews/%d/logs", deploy.ID))
	assertStatus(t, resp, 200)
	if body := string(readBody(t, resp)); !strings.Contains(body, "frontend build") || !strings.Contains(body, "backend build") {
		t.Fatalf("logs = %q", body)
	}

	resp = e.get("/api/previews/999/logs")
	assertStatus(t, resp, 404)
}

// TestAutoDeployOnIdle: an agent reporting idle (finished a work burst)
// triggers a deploy of the branch tip — gated on the worktree carrying a
// preview.toml.
func TestAutoDeployOnIdle(t *testing.T) {
	e := newEnv(t)
	seedPreviewableRepo(t, e)
	board := e.seedBoard("Demo Board")
	ticket := e.seedTicket(board, "Auto feature")
	sess := e.seedSession(ticket)
	mustGit(t, e.repoPath, "branch", sess.BranchName)

	// The gate: no preview.toml in the worktree → idle must NOT deploy.
	if err := os.MkdirAll(sess.WorktreePath, 0o755); err != nil {
		t.Fatal(err)
	}
	resp := e.patch(fmt.Sprintf("/api/sessions/%d/status", sess.ID), map[string]string{"status": "idle"})
	assertStatus(t, resp, 204)
	readBody(t, resp)
	time.Sleep(300 * time.Millisecond)
	resp = e.get(fmt.Sprintf("/api/sessions/%d/previews", sess.ID))
	assertStatus(t, resp, 200)
	if got := decodeJSON[[]orchestrator.Deploy](t, resp); len(got) != 0 {
		t.Fatalf("deploy without preview.toml gate: %+v", got)
	}

	// With preview.toml present, idle deploys the tip.
	if err := os.WriteFile(filepath.Join(sess.WorktreePath, "preview.toml"), []byte(previewManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	resp = e.patch(fmt.Sprintf("/api/sessions/%d/status", sess.ID), map[string]string{"status": "idle"})
	assertStatus(t, resp, 204)
	readBody(t, resp)

	deadline := time.Now().Add(30 * time.Second)
	for {
		resp = e.get(fmt.Sprintf("/api/sessions/%d/previews", sess.ID))
		assertStatus(t, resp, 200)
		deploys := decodeJSON[[]orchestrator.Deploy](t, resp)
		if len(deploys) == 1 && deploys[0].Status == orchestrator.StatusReady {
			break
		}
		if len(deploys) == 1 && deploys[0].Status == orchestrator.StatusFailed {
			t.Fatalf("auto-deploy failed: %s", deploys[0].Error)
		}
		if time.Now().After(deadline) {
			t.Fatalf("auto-deploy never became ready: %+v", deploys)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Re-reporting idle with an unchanged tip is a no-op (idempotent per
	// commit — still exactly one deploy).
	resp = e.patch(fmt.Sprintf("/api/sessions/%d/status", sess.ID), map[string]string{"status": "idle"})
	assertStatus(t, resp, 204)
	readBody(t, resp)
	time.Sleep(300 * time.Millisecond)
	resp = e.get(fmt.Sprintf("/api/sessions/%d/previews", sess.ID))
	deploys := decodeJSON[[]orchestrator.Deploy](t, resp)
	if len(deploys) != 1 {
		t.Fatalf("idempotency: %d deploys", len(deploys))
	}

	// The env kill-switch works.
	t.Setenv("KANBAN_PREVIEW_AUTO_DEPLOY", "0")
	mustGit(t, e.repoPath, "commit", "--allow-empty", "-qm", "new tip")
	mustGit(t, e.repoPath, "branch", "-f", sess.BranchName, "HEAD")
	resp = e.patch(fmt.Sprintf("/api/sessions/%d/status", sess.ID), map[string]string{"status": "idle"})
	assertStatus(t, resp, 204)
	readBody(t, resp)
	time.Sleep(300 * time.Millisecond)
	resp = e.get(fmt.Sprintf("/api/sessions/%d/previews", sess.ID))
	if deploys := decodeJSON[[]orchestrator.Deploy](t, resp); len(deploys) != 1 {
		t.Fatalf("kill-switch ignored: %d deploys", len(deploys))
	}
}

func TestSessionPreviewsRequireRepoAndBranch(t *testing.T) {
	e := newEnv(t)

	// A board without a git repo can't deploy.
	noRepo := &db.Board{Name: "No Repo", Slug: "no-repo"}
	if err := e.store.CreateBoard(t.Context(), noRepo); err != nil {
		t.Fatal(err)
	}
	ticket := e.seedTicket(noRepo, "Task")
	sess := e.seedSession(ticket)

	resp := e.post(fmt.Sprintf("/api/sessions/%d/previews", sess.ID), nil)
	assertStatus(t, resp, 400)

	resp = e.get("/api/sessions/999/previews")
	assertStatus(t, resp, 404)
}
