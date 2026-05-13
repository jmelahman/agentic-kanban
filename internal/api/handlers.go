package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/errreport"
	"github.com/jmelahman/kanban/internal/git"
	gh "github.com/jmelahman/kanban/internal/github"
	"github.com/jmelahman/kanban/internal/harness"
	"github.com/jmelahman/kanban/internal/hooks"
	"github.com/jmelahman/kanban/internal/kanbantoml"
	"github.com/jmelahman/kanban/internal/session"
	"github.com/jmelahman/kanban/internal/tasks"
)

type handlers struct {
	store    *db.Store
	docker   *docker.Client
	sessions *session.Manager
	hooks    *hooks.Runner
	config   *config.Config
	tasks    *tasks.Runner
	bus      *EventBus
	build    BuildInfo
	reporter *errreport.Reporter
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DB().PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"db": err.Error()})
		return
	}
	if err := h.docker.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"docker": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.build)
}

// Boards

type createBoardReq struct {
	Name           string `json:"name"`
	RepoPath       string `json:"repo_path"`
	MountPath      string `json:"mount_path"`
	WorktreeRoot   string `json:"worktree_root"`
	BaseBranch     string `json:"base_branch"`
	BranchPrefix   string `json:"branch_prefix"`
	GitAuthorName  string `json:"git_author_name"`
	GitAuthorEmail string `json:"git_author_email"`
}

func (h *handlers) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := h.store.ListBoards(r.Context())
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, boards)
}

func (h *handlers) createBoard(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBody[createBoardReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.RepoPath = strings.TrimSpace(req.RepoPath)
	req.MountPath = strings.TrimSpace(req.MountPath)
	if req.Name == "" {
		h.httpError(w, fmt.Errorf("name required"), 400)
		return
	}
	if req.RepoPath == "" && req.MountPath == "" {
		h.httpError(w, fmt.Errorf("at least one of repo_path or mount_path required"), 400)
		return
	}
	if req.BaseBranch == "" && req.RepoPath != "" {
		if b, err := git.DefaultBranch(req.RepoPath); err == nil {
			req.BaseBranch = strings.TrimSpace(b)
		}
	}
	if req.BaseBranch == "" {
		req.BaseBranch = "main"
	}
	if req.WorktreeRoot == "" && req.RepoPath != "" {
		req.WorktreeRoot = filepath.Join(h.config.WorktreesDir(), slugify(req.Name))
	}
	board := &db.Board{
		Name:           req.Name,
		Slug:           slugify(req.Name),
		RepoPath:       req.RepoPath,
		MountPath:      req.MountPath,
		WorktreeRoot:   req.WorktreeRoot,
		BaseBranch:     req.BaseBranch,
		BranchPrefix:   strings.TrimSpace(req.BranchPrefix),
		GitAuthorName:  strings.TrimSpace(req.GitAuthorName),
		GitAuthorEmail: strings.TrimSpace(req.GitAuthorEmail),
	}
	if err := h.store.CreateBoard(r.Context(), board); err != nil {
		if isUniqueViolation(err) {
			h.httpError(w, fmt.Errorf("a board named %q already exists", req.Name), http.StatusConflict)
			return
		}
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 201, board)
}

type updateBoardReq struct {
	Name           *string `json:"name"`
	RepoPath       *string `json:"repo_path"`
	MountPath      *string `json:"mount_path"`
	WorktreeRoot   *string `json:"worktree_root"`
	BaseBranch     *string `json:"base_branch"`
	BranchPrefix   *string `json:"branch_prefix"`
	GitAuthorName  *string `json:"git_author_name"`
	GitAuthorEmail *string `json:"git_author_email"`
}

func (h *handlers) updateBoard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	board, err := h.store.GetBoard(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	req, err := decodeBody[updateBoardReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			h.httpError(w, fmt.Errorf("name cannot be empty"), 400)
			return
		}
		board.Name = name
	}
	if req.RepoPath != nil {
		board.RepoPath = strings.TrimSpace(*req.RepoPath)
	}
	if req.MountPath != nil {
		board.MountPath = strings.TrimSpace(*req.MountPath)
	}
	if board.RepoPath == "" && board.MountPath == "" {
		h.httpError(w, fmt.Errorf("at least one of repo_path or mount_path required"), 400)
		return
	}
	if req.WorktreeRoot != nil {
		board.WorktreeRoot = strings.TrimSpace(*req.WorktreeRoot)
	}
	if req.BaseBranch != nil {
		base := strings.TrimSpace(*req.BaseBranch)
		if base == "" {
			h.httpError(w, fmt.Errorf("base_branch cannot be empty"), 400)
			return
		}
		board.BaseBranch = base
	}
	if req.BranchPrefix != nil {
		board.BranchPrefix = strings.TrimSpace(*req.BranchPrefix)
	}
	if req.GitAuthorName != nil {
		board.GitAuthorName = strings.TrimSpace(*req.GitAuthorName)
	}
	if req.GitAuthorEmail != nil {
		board.GitAuthorEmail = strings.TrimSpace(*req.GitAuthorEmail)
	}
	if err := h.store.UpdateBoard(r.Context(), board); err != nil {
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, board)
}

func (h *handlers) deleteBoard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if _, err := h.store.GetBoard(r.Context(), id); err != nil {
		h.httpError(w, err, 404)
		return
	}
	sessions, err := h.store.ListSessionsByBoard(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	for _, sess := range sessions {
		if err := h.sessions.Destroy(r.Context(), sess.ID); err != nil {
			log.Printf("delete board %d: destroy session %d: %v", id, sess.ID, err)
		}
	}
	if err := h.store.DeleteBoard(r.Context(), id); err != nil {
		h.httpError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (h *handlers) getBoard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	board, err := h.store.GetBoard(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	writeJSON(w, 200, board)
}

type boardStateResp struct {
	Board       *db.Board    `json:"board"`
	Columns     []db.Column  `json:"columns"`
	Tickets     []db.Ticket  `json:"tickets"`
	Sessions    []db.Session `json:"sessions"`
	MergeConfig MergeConfig  `json:"merge_config"`
	SyncConfig  SyncConfig   `json:"sync_config"`
}

func (h *handlers) boardState(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	board, err := h.store.GetBoard(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	cols, err := h.store.ListColumns(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	tickets, err := h.store.ListTickets(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	sessions, err := h.store.ListSessionsByBoard(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, boardStateResp{
		Board:       board,
		Columns:     cols,
		Tickets:     tickets,
		Sessions:    sessions,
		MergeConfig: loadMergeConfig(board.RepoPath),
		SyncConfig:  loadSyncConfig(board.RepoPath),
	})
}

// Tickets

type createTicketReq struct {
	ColumnID int64  `json:"column_id,omitempty"`
	Column   string `json:"column,omitempty"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

func (h *handlers) createTicket(w http.ResponseWriter, r *http.Request) {
	board, err := h.resolveBoardIdent(r.Context(), r.PathValue("id"))
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	req, err := decodeBody[createTicketReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if req.Title == "" {
		h.httpError(w, fmt.Errorf("title required"), 400)
		return
	}
	columnID, err := h.resolveColumn(r.Context(), board.ID, req.ColumnID, req.Column)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	t := &db.Ticket{
		BoardID:  board.ID,
		ColumnID: columnID,
		Title:    req.Title,
		Slug:     slugify(req.Title),
		Body:     req.Body,
	}
	if err := h.store.CreateTicket(r.Context(), t); err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.bus.Publish(board.ID, "ticket_created", t)
	h.hooks.Fire(&board.ID, hooks.EventTicketCreated, map[string]string{
		"ticket_id": fmt.Sprintf("%d", t.ID),
		"board":     board.Name,
	})
	writeJSON(w, 201, t)
}

type updateTicketReq struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

func (h *handlers) updateTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	req, err := decodeBody[updateTicketReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			h.httpError(w, fmt.Errorf("title cannot be empty"), 400)
			return
		}
		t.Title = title
	}
	if req.Body != nil {
		t.Body = *req.Body
	}
	if err := h.store.UpdateTicket(r.Context(), t); err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.bus.Publish(t.BoardID, "ticket_updated", t)
	writeJSON(w, 200, t)
}

// resolveBoardIdent looks up a board by numeric id first, then by slug. The
// numeric path stays the canonical form for back-compat; slug is a fallback
// so programmatic callers don't need to pre-resolve IDs.
func (h *handlers) resolveBoardIdent(ctx context.Context, ident string) (*db.Board, error) {
	if ident == "" {
		return nil, fmt.Errorf("board identifier required")
	}
	if id, err := strconv.ParseInt(ident, 10, 64); err == nil {
		if b, err := h.store.GetBoard(ctx, id); err == nil {
			return b, nil
		}
	}
	return h.store.GetBoardBySlug(ctx, ident)
}

// resolveColumn picks a column for a new ticket. Precedence: numeric column_id
// > column (numeric string or case-insensitive name) > leftmost column.
func (h *handlers) resolveColumn(ctx context.Context, boardID int64, columnID int64, column string) (int64, error) {
	cols, err := h.store.ListColumns(ctx, boardID)
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, fmt.Errorf("board has no columns")
	}
	if columnID > 0 {
		for _, c := range cols {
			if c.ID == columnID {
				return c.ID, nil
			}
		}
		return 0, fmt.Errorf("column_id %d not found on this board", columnID)
	}
	if column != "" {
		if id, err := strconv.ParseInt(column, 10, 64); err == nil {
			for _, c := range cols {
				if c.ID == id {
					return c.ID, nil
				}
			}
		}
		for _, c := range cols {
			if strings.EqualFold(c.Name, column) {
				return c.ID, nil
			}
		}
		return 0, fmt.Errorf("column %q not found on this board", column)
	}
	return cols[0].ID, nil
}

type moveTicketReq struct {
	ColumnID int64 `json:"column_id"`
	Position int   `json:"position"`
}

func (h *handlers) moveTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	req, err := decodeBody[moveTicketReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if err := h.store.MoveTicket(r.Context(), id, req.ColumnID, req.Position); err != nil {
		h.httpError(w, err, 500)
		return
	}
	t, _ := h.store.GetTicket(r.Context(), id)
	if t != nil {
		h.bus.Publish(t.BoardID, "ticket_moved", t)
	}
	w.WriteHeader(204)
}

func (h *handlers) archiveTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	h.stopSessionForTicket(r.Context(), id)
	if err := h.store.ArchiveTicket(r.Context(), id); err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.publishTicketArchived(t)
	w.WriteHeader(204)
}

func (h *handlers) unarchiveTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	if t.ArchivedAt == nil {
		h.httpError(w, fmt.Errorf("ticket is not archived"), 400)
		return
	}
	if err := h.store.UnarchiveTicket(r.Context(), id); err != nil {
		h.httpError(w, err, 500)
		return
	}
	updated, _ := h.store.GetTicket(r.Context(), id)
	if updated != nil {
		h.bus.Publish(updated.BoardID, "ticket_unarchived", updated)
	}
	w.WriteHeader(204)
}

func (h *handlers) listArchivedTickets(w http.ResponseWriter, r *http.Request) {
	boardID := pathID(r, "id")
	tickets, err := h.store.ListArchivedTickets(r.Context(), boardID)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, tickets)
}

func (h *handlers) deleteTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	if t.ArchivedAt == nil {
		h.httpError(w, fmt.Errorf("ticket must be archived before deletion"), 400)
		return
	}
	h.destroySessionForTicket(r.Context(), id)
	if err := h.store.DeleteTicket(r.Context(), id); err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.bus.Publish(t.BoardID, "ticket_deleted", t)
	w.WriteHeader(204)
}

// archiveColumnTickets archives every non-archived ticket in a column. Mirrors
// archiveTicket per-ticket: stops any running session, archives, then publishes
// a ticket_archived SSE + EventTicketArchived hook for each ticket affected.
func (h *handlers) archiveColumnTickets(w http.ResponseWriter, r *http.Request) {
	columnID := pathID(r, "id")
	tickets, err := h.store.ListTicketsInColumn(r.Context(), columnID)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	for _, t := range tickets {
		h.stopSessionForTicket(r.Context(), t.ID)
	}
	if _, err := h.store.ArchiveTicketsInColumn(r.Context(), columnID); err != nil {
		h.httpError(w, err, 500)
		return
	}
	for i := range tickets {
		h.publishTicketArchived(&tickets[i])
	}
	w.WriteHeader(204)
}

// deleteAllArchived permanently deletes every archived ticket on a board.
// Mirrors deleteTicket per-ticket: destroys any associated session, deletes
// the ticket, then publishes a ticket_deleted SSE for each.
func (h *handlers) deleteAllArchived(w http.ResponseWriter, r *http.Request) {
	boardID := pathID(r, "id")
	tickets, err := h.store.ListArchivedTickets(r.Context(), boardID)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	for _, t := range tickets {
		h.destroySessionForTicket(r.Context(), t.ID)
	}
	if _, err := h.store.DeleteAllArchivedTickets(r.Context(), boardID); err != nil {
		h.httpError(w, err, 500)
		return
	}
	for i := range tickets {
		t := tickets[i]
		h.bus.Publish(boardID, "ticket_deleted", &t)
	}
	w.WriteHeader(204)
}

type strategyReq struct {
	Strategy string `json:"strategy"`
}

func (h *handlers) syncTicket(w http.ResponseWriter, r *http.Request) {
	h.ticketStrategyAction(w, r,
		[]string{"rebase", "merge"}, "rebase",
		func(repo, strat string) bool { return loadSyncConfig(repo).allows(strat) },
		h.sessions.Sync,
		true,
	)
}

func (h *handlers) mergeTicket(w http.ResponseWriter, r *http.Request) {
	h.ticketStrategyAction(w, r,
		[]string{"merge-commit", "squash", "rebase"}, "",
		func(repo, strat string) bool { return loadMergeConfig(repo).allows(strat) },
		h.sessions.Merge,
		false,
	)
}

// ticketStrategyAction is the shared body of syncTicket and mergeTicket: parse
// strategy, validate against the allowed set and the per-board config, ensure
// a session exists, run the action, optionally publish session_updated.
func (h *handlers) ticketStrategyAction(
	w http.ResponseWriter, r *http.Request,
	allowed []string, defaultStrategy string,
	isAllowed func(repoPath, strategy string) bool,
	action func(ctx context.Context, sessID int64, strategy string) error,
	publishOnSuccess bool,
) {
	id := pathID(r, "id")
	req, err := decodeBody[strategyReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if req.Strategy == "" && defaultStrategy != "" {
		req.Strategy = defaultStrategy
	}
	valid := false
	for _, s := range allowed {
		if s == req.Strategy {
			valid = true
			break
		}
	}
	if !valid {
		h.httpError(w, fmt.Errorf("strategy must be %s", joinStrategies(allowed)), 400)
		return
	}
	_, board, code, err := h.ticketBoard(r.Context(), id)
	if err != nil {
		h.httpError(w, err, code)
		return
	}
	if !isAllowed(board.RepoPath, req.Strategy) {
		h.httpError(w, fmt.Errorf("strategy %s is disabled for this board", req.Strategy), 400)
		return
	}
	sess, err := h.store.GetSessionByTicket(r.Context(), id)
	if err != nil || sess == nil {
		h.httpError(w, fmt.Errorf("no session for ticket"), 404)
		return
	}
	if err := action(r.Context(), sess.ID, req.Strategy); err != nil {
		h.httpError(w, err, 409)
		return
	}
	if publishOnSuccess {
		h.publishSessionUpdated(r.Context(), sess.ID)
	}
	w.WriteHeader(204)
}

// joinStrategies renders an allowed-strategy slice as English ("a or b",
// "a, b, or c") for the strategy-validation error message.
func joinStrategies(s []string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0]
	case 2:
		return s[0] + " or " + s[1]
	default:
		return strings.Join(s[:len(s)-1], ", ") + ", or " + s[len(s)-1]
	}
}

func (h *handlers) doneTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	doneCol, err := h.store.FindColumnByName(r.Context(), t.BoardID, "Done")
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			h.httpError(w, fmt.Errorf("board has no Done column"), 409)
			return
		}
		h.httpError(w, err, 500)
		return
	}
	if sess, err := h.store.GetSessionByTicket(r.Context(), id); err == nil && sess != nil {
		if err := h.sessions.Stop(r.Context(), sess.ID); err != nil {
			log.Printf("done: stop session %d: %v", sess.ID, err)
		}
	}
	maxPos, err := h.store.MaxTicketPosition(r.Context(), doneCol.ID)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	if err := h.store.MoveTicket(r.Context(), t.ID, doneCol.ID, maxPos+1); err != nil {
		h.httpError(w, err, 500)
		return
	}
	if updated, _ := h.store.GetTicket(r.Context(), id); updated != nil {
		h.bus.Publish(updated.BoardID, "ticket_moved", updated)
	}
	w.WriteHeader(204)
}

// Sessions

func (h *handlers) ensureSession(w http.ResponseWriter, r *http.Request) {
	ticketID := pathID(r, "id")
	t, board, code, err := h.ticketBoard(r.Context(), ticketID)
	if err != nil {
		h.httpError(w, err, code)
		return
	}
	sess, err := h.sessions.Ensure(r.Context(), board, t)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.publishSessionUpdated(r.Context(), sess.ID)
	writeJSON(w, 201, sess)
}

func (h *handlers) startSession(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.sessions.Start(r.Context(), id, h.makePullProgressCb(r.Context(), id))
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.publishSessionUpdated(r.Context(), sess.ID)
	writeJSON(w, 200, sess)
}

func (h *handlers) restartSession(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.sessions.Restart(r.Context(), id, h.makePullProgressCb(r.Context(), id))
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.publishSessionUpdated(r.Context(), sess.ID)
	writeJSON(w, 200, sess)
}

// makePullProgressCb returns a callback that publishes session_pull_progress
// SSE events to the session's board. Returns nil if the session or its ticket
// can't be resolved — Start will surface that as its own error.
func (h *handlers) makePullProgressCb(ctx context.Context, sessionID int64) docker.PullProgressFunc {
	sess, err := h.store.GetSession(ctx, sessionID)
	if err != nil || sess == nil {
		return nil
	}
	t, err := h.store.GetTicket(ctx, sess.TicketID)
	if err != nil || t == nil {
		return nil
	}
	boardID := t.BoardID
	return func(p docker.PullProgress) {
		h.bus.Publish(boardID, "session_pull_progress", map[string]any{
			"session_id": sessionID,
			"image":      p.Image,
			"current":    p.Current,
			"total":      p.Total,
			"layers":     p.Layers,
			"status":     p.Status,
			"done":       p.Done,
		})
	}
}

func (h *handlers) stopSession(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := h.sessions.Stop(r.Context(), id); err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.publishSessionUpdated(r.Context(), id)
	w.WriteHeader(204)
}

// stopSessionForTicket best-effort stops any session attached to ticketID.
// No-op when no session exists; lookup or stop errors are swallowed since
// callers use it as fire-and-forget before archiving.
func (h *handlers) stopSessionForTicket(ctx context.Context, ticketID int64) {
	if sess, err := h.store.GetSessionByTicket(ctx, ticketID); err == nil && sess != nil {
		_ = h.sessions.Stop(ctx, sess.ID)
	}
}

// destroySessionForTicket is the delete-time counterpart of
// stopSessionForTicket: it tears down container + worktree rather than
// just stopping the agent.
func (h *handlers) destroySessionForTicket(ctx context.Context, ticketID int64) {
	if sess, err := h.store.GetSessionByTicket(ctx, ticketID); err == nil && sess != nil {
		_ = h.sessions.Destroy(ctx, sess.ID)
	}
}

// publishTicketArchived emits the "ticket_archived" SSE and fires the
// EventTicketArchived hook in one go — the standard pair after any
// archive-ticket store mutation.
func (h *handlers) publishTicketArchived(t *db.Ticket) {
	h.bus.Publish(t.BoardID, "ticket_archived", t)
	h.hooks.Fire(&t.BoardID, hooks.EventTicketArchived, map[string]string{
		"ticket_id": fmt.Sprintf("%d", t.ID),
	})
}

// publishSessionUpdated emits a "session_updated" event on the board's
// channel. Always refetches the session from the DB before publishing
// so concurrent writes (e.g. the GitHub PR poller writing pr_number /
// pr_url while a handler is mid-mutation) are reflected on the wire.
// Publishing a stale in-memory snapshot is what caused PR fields to
// disappear from the live UI until the page was refreshed.
// Best-effort: silently no-ops if the session or its ticket can't be
// resolved so callers can use it as fire-and-forget.
func (h *handlers) publishSessionUpdated(ctx context.Context, sessionID int64) {
	sess, err := h.store.GetSession(ctx, sessionID)
	if err != nil || sess == nil {
		return
	}
	t, err := h.store.GetTicket(ctx, sess.TicketID)
	if err != nil || t == nil {
		return
	}
	h.bus.Publish(t.BoardID, "session_updated", sess)
}

type updateSessionStatusReq struct {
	Status string `json:"status"`
}

// updateSessionStatus is called by Claude Code hooks running inside the session
// container to report the active state of the agent (working/idle/awaiting_perm).
// Other statuses (stopped/starting/error) are owned by the session manager and
// rejected here.
func (h *handlers) updateSessionStatus(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	req, err := decodeBody[updateSessionStatusReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	var hookEvent string
	switch req.Status {
	case db.SessionStatusWorking:
		hookEvent = hooks.EventSessionWorking
	case db.SessionStatusIdle:
		hookEvent = hooks.EventSessionIdle
	case db.SessionStatusAwaitingPerm:
		hookEvent = hooks.EventSessionAwaitingPerm
	default:
		h.httpError(w, fmt.Errorf("status must be working, idle, or awaiting_perm"), 400)
		return
	}
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	if err := h.store.UpdateSessionStatus(r.Context(), id, req.Status); err != nil {
		h.httpError(w, err, 500)
		return
	}
	h.publishSessionUpdated(r.Context(), sess.ID)
	if t, _ := h.store.GetTicket(r.Context(), sess.TicketID); t != nil {
		boardID := t.BoardID
		h.hooks.Fire(&boardID, hookEvent, map[string]string{
			"session_id": fmt.Sprintf("%d", sess.ID),
			"ticket_id":  fmt.Sprintf("%d", sess.TicketID),
		})
	}
	w.WriteHeader(204)
}

type updateClaudeSessionIDReq struct {
	ClaudeSessionID string `json:"claude_session_id"`
}

// claudeSessionIDPattern matches the UUID format Claude Code writes for its
// per-conversation transcript filenames (`~/.claude/projects/.../<uuid>.jsonl`).
// Strict-ish: hyphenated 8-4-4-4-12 hex, case-insensitive. Anything else is a
// hook misconfiguration and we reject it so we never persist a value we can't
// later pass to `claude --resume`.
var claudeSessionIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// updateClaudeSessionID is called by the Claude Code SessionStart hook running
// inside the session container to report the UUID of the active conversation.
// Stored so subsequent `claude` launches resume the same transcript across
// container/Kanban restarts. Same trust model as updateSessionStatus — the
// session is identified by URL path, no auth.
func (h *handlers) updateClaudeSessionID(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	req, err := decodeBody[updateClaudeSessionIDReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	uuid := strings.TrimSpace(req.ClaudeSessionID)
	if !claudeSessionIDPattern.MatchString(uuid) {
		h.httpError(w, fmt.Errorf("claude_session_id must be a UUID"), 400)
		return
	}
	if err := h.store.UpdateClaudeSessionID(r.Context(), id, uuid); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			h.httpError(w, err, 404)
			return
		}
		h.httpError(w, err, 500)
		return
	}
	// Push the updated session out over SSE so the InfoPanel's "Claude
	// session" row reflects the UUID without a page reload. Without this,
	// stores seeded from a stale boardState fetch keep displaying the
	// pre-UUID session and the "resume" plumbing looks broken to users.
	h.publishSessionUpdated(r.Context(), id)
	w.WriteHeader(204)
}

// Tasks

type taskInfo struct {
	tasks.VSCodeTask
	ContainerPort int  `json:"container_port,omitempty"`
	HasPort       bool `json:"has_port"`
}

type discoverTasksResp struct {
	Tasks    []taskInfo `json:"tasks"`
	Warnings []string   `json:"warnings"`
}

func (h *handlers) discoverTasks(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	found, warnings, _ := tasks.Discover(sess.WorktreePath)
	out := make([]taskInfo, 0, len(found))
	for _, t := range found {
		info := taskInfo{VSCodeTask: t}
		if port, ok := tasks.PortFor(sess.WorktreePath, t.Label); ok {
			info.ContainerPort = port
			info.HasPort = true
		}
		out = append(out, info)
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, 200, discoverTasksResp{Tasks: out, Warnings: warnings})
}

func (h *handlers) listTaskRuns(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	runs, err := h.store.ListTaskRuns(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, runs)
}

type createTaskRunReq struct {
	Label string `json:"label"`
}

func (h *handlers) createTaskRun(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	req, err := decodeBody[createTaskRunReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	found, _, err := tasks.Discover(sess.WorktreePath)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	var task tasks.VSCodeTask
	for _, t := range found {
		if t.Label == req.Label {
			task = t
			break
		}
	}
	if task.Label == "" {
		h.httpError(w, fmt.Errorf("task %q not found", req.Label), 404)
		return
	}
	tr, err := h.tasks.Start(r.Context(), sess, task)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}

	if port, ok := tasks.PortFor(sess.WorktreePath, task.Label); ok {
		_ = h.ensurePortProxy(r.Context(), sess, task.Label, port)
	}

	writeJSON(w, 201, tr)
}

func (h *handlers) stopTaskRun(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	tr, err := h.store.GetTaskRun(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	sess, err := h.store.GetSession(r.Context(), tr.SessionID)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	if err := h.tasks.Stop(r.Context(), sess, tr); err != nil {
		h.httpError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (h *handlers) taskRunOutput(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.httpError(w, fmt.Errorf("streaming unsupported"), 500)
		return
	}
	writeSSEHeaders(w)

	ch, cancel := h.tasks.Subscribe(id)
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case s, ok := <-ch:
			if !ok {
				fmt.Fprintf(w, "event: end\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", strings.TrimRight(s, "\n"))
			flusher.Flush()
		}
	}
}

// Ports

func (h *handlers) listPorts(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	ports, err := h.store.ListPorts(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, ports)
}

type createPortReq struct {
	Label         string `json:"label"`
	ContainerPort int    `json:"container_port"`
}

func (h *handlers) createPort(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	req, err := decodeBody[createPortReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if req.ContainerPort <= 0 {
		h.httpError(w, fmt.Errorf("container_port required"), 400)
		return
	}
	if err := h.ensurePortProxy(r.Context(), sess, req.Label, req.ContainerPort); err != nil {
		h.httpError(w, err, 500)
		return
	}
	ports, _ := h.store.ListPorts(r.Context(), id)
	writeJSON(w, 201, ports)
}

func (h *handlers) deletePort(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if p, err := h.store.GetPort(r.Context(), id); err == nil && p.ProxyActive {
		h.sessions.Proxies().Close(p.HostPort)
	}
	if err := h.store.DeletePort(r.Context(), id); err != nil {
		h.httpError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (h *handlers) prDetail(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	if sess.PRNumber == nil || *sess.PRNumber == 0 {
		h.httpError(w, fmt.Errorf("session has no pull request"), 404)
		return
	}
	repoPath := sess.RepoPath
	if repoPath == "" {
		board, err := h.boardForSession(r.Context(), sess)
		if err != nil {
			h.httpError(w, err, 500)
			return
		}
		repoPath = board.RepoPath
	}
	if repoPath == "" {
		h.httpError(w, fmt.Errorf("no repo path for session"), 400)
		return
	}
	detail, err := gh.FetchPRDetail(r.Context(), repoPath, *sess.PRNumber)
	if err != nil {
		h.httpError(w, err, 502)
		return
	}
	writeJSON(w, 200, detail)
}

func (h *handlers) ensurePortProxy(ctx context.Context, sess *db.Session, label string, containerPort int) error {
	existing, _ := h.store.ListPorts(ctx, sess.ID)
	for _, p := range existing {
		if p.ContainerPort == containerPort {
			if !p.ProxyActive {
				if err := h.startProxy(ctx, sess, p); err != nil {
					return err
				}
			}
			return nil
		}
	}
	hostPort, err := h.store.AllocateHostPort(ctx, h.config.PortRangeStart, h.config.PortRangeEnd)
	if err != nil {
		return err
	}
	p := &db.PortAllocation{SessionID: sess.ID, Label: label, ContainerPort: containerPort, HostPort: hostPort}
	if err := h.store.CreatePort(ctx, p); err != nil {
		return err
	}
	return h.startProxy(ctx, sess, *p)
}

func (h *handlers) startProxy(ctx context.Context, sess *db.Session, p db.PortAllocation) error {
	if sess.ContainerID == nil || *sess.ContainerID == "" {
		return fmt.Errorf("session not running")
	}
	if err := h.sessions.Proxies().Open(p.HostPort, *sess.ContainerID, p.ContainerPort); err != nil {
		return err
	}
	if err := h.store.SetPortActive(ctx, p.ID, true); err != nil {
		return err
	}
	board, _ := h.boardForSession(ctx, sess)
	var boardID *int64
	if board != nil {
		boardID = &board.ID
	}
	h.hooks.Fire(boardID, hooks.EventPortExposed, map[string]string{
		"session_id":     fmt.Sprintf("%d", sess.ID),
		"label":          p.Label,
		"container_port": fmt.Sprintf("%d", p.ContainerPort),
		"host_port":      fmt.Sprintf("%d", p.HostPort),
	})
	return nil
}

func (h *handlers) boardForSession(ctx context.Context, sess *db.Session) (*db.Board, error) {
	_, board, _, err := h.ticketBoard(ctx, sess.TicketID)
	return board, err
}

// ticketBoard fetches a ticket and its parent board in one go. The returned
// status is the HTTP code callers should pass to httpError when err != nil:
// 404 if the ticket lookup failed (typically ErrNotFound), 500 otherwise.
func (h *handlers) ticketBoard(ctx context.Context, id int64) (*db.Ticket, *db.Board, int, error) {
	t, err := h.store.GetTicket(ctx, id)
	if err != nil {
		return nil, nil, 404, err
	}
	board, err := h.store.GetBoard(ctx, t.BoardID)
	if err != nil {
		return nil, nil, 500, err
	}
	return t, board, 0, nil
}

// PTY

func (h *handlers) wsPTY(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	repoPath := ""
	if board, err := h.boardForSession(r.Context(), sess); err == nil && board != nil {
		repoPath = board.RepoPath
	}
	resolved := harness.Resolve(repoPath)
	cmd := resolved.PTYCommand
	// Resume the prior Claude Code conversation when we have its UUID. The
	// SessionStart hook in .claude/settings.local.json captures the UUID on
	// every launch, so this preserves the conversation across container
	// restarts. Other harnesses don't have an equivalent flag, so this is
	// gated on the resolved harness ID.
	if resolved.ID == "claude" && sess.ClaudeSessionID != "" {
		cmd = append(append([]string(nil), cmd...), "--resume", sess.ClaudeSessionID)
	}
	_ = h.sessions.AttachAgent(r.Context(), sess, w, r, cmd, "/workspace")
}

func (h *handlers) wsShell(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		h.httpError(w, err, 404)
		return
	}
	_ = h.sessions.AttachShell(r.Context(), sess, w, r, "/workspace")
}

// Settings — backed by the user-level config file at
// $XDG_CONFIG_HOME/kanban/config.toml. Empty values mean "no user override".

type settingsResp struct {
	Harness               string `json:"harness"`
	WorktreesRoot         string `json:"worktrees_root"`
	WorktreesRootResolved string `json:"worktrees_root_resolved"`
	WorktreesRootLocked   bool   `json:"worktrees_root_locked"`
}

func (h *handlers) settingsResponse() settingsResp {
	id, _ := harness.ReadUserHarness()
	return settingsResp{
		Harness:               id,
		WorktreesRoot:         readUserWorktreesRoot(),
		WorktreesRootResolved: h.config.WorktreesDir(),
		WorktreesRootLocked:   h.config.HasWorktreesDirOverride(),
	}
}

func (h *handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, h.settingsResponse())
}

type updateSettingsReq struct {
	Harness       *string `json:"harness"`
	WorktreesRoot *string `json:"worktrees_root"`
}

func (h *handlers) updateSettings(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBody[updateSettingsReq](r)
	if err != nil {
		h.httpError(w, err, 400)
		return
	}
	if req.Harness != nil {
		id := *req.Harness
		if id != "" && !harness.IsKnown(id) {
			h.httpError(w, fmt.Errorf("unknown harness %q", id), 400)
			return
		}
		if err := harness.WriteUserHarness(id); err != nil {
			h.httpError(w, err, 500)
			return
		}
	}
	if req.WorktreesRoot != nil {
		if h.config.HasWorktreesDirOverride() {
			h.httpError(w, fmt.Errorf("worktrees root is locked by --worktrees-dir or $KANBAN_WORKTREES_DIR"), http.StatusConflict)
			return
		}
		root := strings.TrimSpace(*req.WorktreesRoot)
		if err := kanbantoml.WriteUserWorktreesRoot(root); err != nil {
			h.httpError(w, err, 500)
			return
		}
	}
	writeJSON(w, 200, h.settingsResponse())
}

func readUserWorktreesRoot() string {
	f := kanbantoml.Load("")
	if f.Worktrees == nil || f.Worktrees.Root == nil {
		return ""
	}
	return *f.Worktrees.Root
}

func (h *handlers) listHarnesses(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, harness.Registry)
}

// helpers

func pathID(r *http.Request, name string) int64 {
	v := r.PathValue(name)
	id, _ := strconv.ParseInt(v, 10, 64)
	return id
}

// decodeBody is the single funnel for JSON request bodies. Centralizing it
// keeps any future hardening (size limits, content-type checks, strict
// field validation) in one place rather than 12 handlers.
func decodeBody[T any](r *http.Request) (T, error) {
	var v T
	err := json.NewDecoder(r.Body).Decode(&v)
	return v, err
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeSSEHeaders sets the standard Server-Sent Events response headers.
// Call before the first Flush on any SSE handler.
func writeSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// httpError writes a JSON error response and, for 5xx codes, files a kanban
// ticket via the reporter (a no-op when the reporter is disabled or nil).
// 4xx codes are user/client errors and are not reported.
func (h *handlers) httpError(w http.ResponseWriter, err error, code int) {
	log.Printf("http %d: %v", code, err)
	writeJSON(w, code, map[string]string{"error": err.Error()})
	if code >= 500 && h.reporter != nil {
		h.reporter.Capture(context.Background(), "http", err)
	}
}

type reportFrontendErrorReq struct {
	Message   string            `json:"message"`
	Stack     string            `json:"stack"`
	Source    string            `json:"source"`
	URL       string            `json:"url"`
	UserAgent string            `json:"user_agent"`
	Meta      map[string]string `json:"meta"`
}

// reportFrontendError accepts error reports from the React app's
// ErrorBoundary, global React Query onError, window error/unhandledrejection
// listeners, and the long-task observer. Always returns 204; when the reporter
// is disabled (the default), this endpoint silently absorbs reports.
func (h *handlers) reportFrontendError(w http.ResponseWriter, r *http.Request) {
	req, err := decodeBody[reportFrontendErrorReq](r)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.reporter != nil && req.Message != "" {
		source := strings.TrimSpace(req.Source)
		if source == "" {
			source = "frontend"
		}
		meta := map[string]string{
			"url":        req.URL,
			"user_agent": req.UserAgent,
		}
		for k, v := range req.Meta {
			if _, taken := meta[k]; taken {
				continue
			}
			meta[k] = v
		}
		h.reporter.Report(r.Context(), source, req.Message, req.Stack, meta)
	}
	w.WriteHeader(http.StatusNoContent)
}

// fsCheck reports whether a host path is visible to the kanban server and, if
// so, whether it looks like a git repo. Paths outside the kanban container's
// mounts return "unknown" — they may still be valid host paths that dockerd
// can mount; we just can't see them from here.
func (h *handlers) fsCheck(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		h.httpError(w, fmt.Errorf("path required"), 400)
		return
	}
	state := "unknown"
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			if g, err := os.Stat(filepath.Join(path, ".git")); err == nil && (g.IsDir() || g.Mode().IsRegular()) {
				state = "git"
			} else {
				state = "not_git"
			}
		} else {
			state = "not_git"
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"state": state})
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// slugMaxLen caps slugified output so that branch names built as
// "<prefix>/<board-slug>/<ticket-slug>" stay under GitHub's ~244-char
// branch-name limit even with the longest expected prefix.
const slugMaxLen = 80

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
			prevDash = false
		case c == '-' || c == '_':
			b.WriteRune('-')
			prevDash = true
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > slugMaxLen {
		out = strings.TrimRight(out[:slugMaxLen], "-")
	}
	if out == "" {
		out = "x"
	}
	return out
}
