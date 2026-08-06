package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jmelahman/local-preview/orchestrator"

	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/session"
)

// Preview deployments: each ticket branch can be deployed as a live preview
// served at <sha>.<board>.<preview-domain> by the embedded local-preview
// orchestrator. Deploys build from the committed tree of the board repo's
// branch (worktree branches share the repo's object store, so agent commits
// are visible with no extra plumbing).

// previewRepoName derives the orchestrator repo name — a DNS label, since it
// becomes the subdomain segment — from the board slug.
func previewRepoName(b *db.Board) string {
	var sb strings.Builder
	for _, c := range strings.ToLower(b.Slug) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			sb.WriteRune(c)
		}
	}
	name := strings.Trim(sb.String(), "-")
	if name == "" {
		return fmt.Sprintf("board-%d", b.ID)
	}
	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}
	return name
}

// previewContext resolves the session-scoped preview inputs: the session,
// its board, the orchestrator repo name, and the host repo path. Writes the
// error response and returns ok=false on failure.
func (h *handlers) previewContext(w http.ResponseWriter, r *http.Request) (sess *db.Session, repoName, repoPath string, ok bool) {
	if h.previews == nil {
		h.httpError(w, fmt.Errorf("preview orchestrator unavailable (see server logs)"), 503)
		return nil, "", "", false
	}
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return nil, "", "", false
	}
	board, err := h.boardForSession(r.Context(), sess)
	if err != nil {
		h.httpError(w, err, 500)
		return nil, "", "", false
	}
	paths := session.ResolvePaths(board, sess)
	if !paths.HasRepo {
		h.httpError(w, fmt.Errorf("board has no git repo to deploy"), 400)
		return nil, "", "", false
	}
	if sess.BranchName == "" {
		h.httpError(w, fmt.Errorf("session has no branch to deploy"), 400)
		return nil, "", "", false
	}
	return sess, previewRepoName(board), paths.RepoPath, true
}

// listSessionPreviews returns the session branch's preview deploys, newest
// first. A board never deployed before simply has none.
func (h *handlers) listSessionPreviews(w http.ResponseWriter, r *http.Request) {
	sess, repoName, _, ok := h.previewContext(w, r)
	if !ok {
		return
	}
	all, err := h.previews.Deploys(repoName)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	deploys := []orchestrator.Deploy{}
	for _, d := range all {
		if d.Ref == sess.BranchName {
			deploys = append(deploys, d)
		}
	}
	writeJSON(w, 200, deploys)
}

// createSessionPreview registers the board repo with the orchestrator
// (idempotent) and requests a deploy of the session branch's current tip.
func (h *handlers) createSessionPreview(w http.ResponseWriter, r *http.Request) {
	sess, repoName, repoPath, ok := h.previewContext(w, r)
	if !ok {
		return
	}
	if _, err := h.previews.RegisterRepo(r.Context(), repoName, repoPath); err != nil {
		h.httpError(w, fmt.Errorf("register repo for previews: %w", err), 500)
		return
	}
	deploy, err := h.previews.RequestDeploy(r.Context(), repoName, sess.BranchName, false)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	writeJSON(w, 202, deploy)
}

// previewLogs returns the build-log snapshot for one preview deploy.
func (h *handlers) previewLogs(w http.ResponseWriter, r *http.Request) {
	if h.previews == nil {
		h.httpError(w, fmt.Errorf("preview orchestrator unavailable (see server logs)"), 503)
		return
	}
	logs, err := h.previews.DeployLogs(pathID(r, "id"))
	if errors.Is(err, orchestrator.ErrNotFound) {
		h.httpError(w, fmt.Errorf("preview not found"), 404)
		return
	}
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, logs)
}
