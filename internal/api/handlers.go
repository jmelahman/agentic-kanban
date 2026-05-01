package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jmelahman/kanban/internal/config"
	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/harness"
	"github.com/jmelahman/kanban/internal/hooks"
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
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	if err := h.docker.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"docker": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Boards

type createBoardReq struct {
	Name           string `json:"name"`
	SourceRepoPath string `json:"source_repo_path"`
	WorktreeRoot   string `json:"worktree_root"`
	BaseBranch     string `json:"base_branch"`
}

func (h *handlers) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := h.store.ListBoards(r.Context())
	if err != nil {
		httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, boards)
}

func (h *handlers) createBoard(w http.ResponseWriter, r *http.Request) {
	var req createBoardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if req.Name == "" || req.SourceRepoPath == "" {
		httpError(w, fmt.Errorf("name and source_repo_path required"), 400)
		return
	}
	if req.BaseBranch == "" {
		req.BaseBranch = "main"
	}
	if req.WorktreeRoot == "" {
		req.WorktreeRoot = filepath.Join(h.config.WorktreesDir(), slugify(req.Name))
	}
	board := &db.Board{
		Name:           req.Name,
		Slug:           slugify(req.Name),
		SourceRepoPath: req.SourceRepoPath,
		WorktreeRoot:   req.WorktreeRoot,
		BaseBranch:     req.BaseBranch,
	}
	if err := h.store.CreateBoard(r.Context(), board); err != nil {
		if isUniqueViolation(err) {
			httpError(w, fmt.Errorf("a board named %q already exists", req.Name), http.StatusConflict)
			return
		}
		httpError(w, err, 500)
		return
	}
	writeJSON(w, 201, board)
}

type updateBoardReq struct {
	Name           *string `json:"name"`
	SourceRepoPath *string `json:"source_repo_path"`
	WorktreeRoot   *string `json:"worktree_root"`
	BaseBranch     *string `json:"base_branch"`
}

func (h *handlers) updateBoard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	board, err := h.store.GetBoard(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	var req updateBoardReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			httpError(w, fmt.Errorf("name cannot be empty"), 400)
			return
		}
		board.Name = name
	}
	if req.SourceRepoPath != nil {
		path := strings.TrimSpace(*req.SourceRepoPath)
		if path == "" {
			httpError(w, fmt.Errorf("source_repo_path cannot be empty"), 400)
			return
		}
		board.SourceRepoPath = path
	}
	if req.WorktreeRoot != nil {
		board.WorktreeRoot = *req.WorktreeRoot
	}
	if req.BaseBranch != nil {
		base := strings.TrimSpace(*req.BaseBranch)
		if base == "" {
			httpError(w, fmt.Errorf("base_branch cannot be empty"), 400)
			return
		}
		board.BaseBranch = base
	}
	if err := h.store.UpdateBoard(r.Context(), board); err != nil {
		httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, board)
}

func (h *handlers) deleteBoard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if _, err := h.store.GetBoard(r.Context(), id); err != nil {
		httpError(w, err, 404)
		return
	}
	sessions, err := h.store.ListSessionsByBoard(r.Context(), id)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	for _, sess := range sessions {
		if err := h.sessions.Destroy(r.Context(), sess.ID); err != nil {
			log.Printf("delete board %d: destroy session %d: %v", id, sess.ID, err)
		}
	}
	if err := h.store.DeleteBoard(r.Context(), id); err != nil {
		httpError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (h *handlers) getBoard(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	board, err := h.store.GetBoard(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
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
		httpError(w, err, 404)
		return
	}
	cols, err := h.store.ListColumns(r.Context(), id)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	tickets, err := h.store.ListTickets(r.Context(), id)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	sessions, err := h.store.ListSessionsByBoard(r.Context(), id)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, boardStateResp{
		Board:       board,
		Columns:     cols,
		Tickets:     tickets,
		Sessions:    sessions,
		MergeConfig: loadMergeConfig(board.SourceRepoPath),
		SyncConfig:  loadSyncConfig(board.SourceRepoPath),
	})
}

// Tickets

type createTicketReq struct {
	ColumnID int64  `json:"column_id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

func (h *handlers) createTicket(w http.ResponseWriter, r *http.Request) {
	boardID := pathID(r, "id")
	board, err := h.store.GetBoard(r.Context(), boardID)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	var req createTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if req.Title == "" || req.ColumnID == 0 {
		httpError(w, fmt.Errorf("title and column_id required"), 400)
		return
	}
	t := &db.Ticket{
		BoardID:  boardID,
		ColumnID: req.ColumnID,
		Title:    req.Title,
		Slug:     slugify(req.Title),
		Body:     req.Body,
	}
	if err := h.store.CreateTicket(r.Context(), t); err != nil {
		httpError(w, err, 500)
		return
	}
	h.bus.Publish(boardID, "ticket_created", t)
	h.hooks.Fire(&board.ID, hooks.EventTicketCreated, map[string]string{
		"ticket_id": fmt.Sprintf("%d", t.ID),
		"board":    board.Name,
	})
	if sess, err := h.sessions.Ensure(r.Context(), board, t); err == nil {
		h.bus.Publish(boardID, "session_updated", sess)
	}
	writeJSON(w, 201, t)
}

type moveTicketReq struct {
	ColumnID int64 `json:"column_id"`
	Position int   `json:"position"`
}

func (h *handlers) moveTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	var req moveTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if err := h.store.MoveTicket(r.Context(), id, req.ColumnID, req.Position); err != nil {
		httpError(w, err, 500)
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
		httpError(w, err, 404)
		return
	}
	if sess, err := h.store.GetSessionByTicket(r.Context(), id); err == nil && sess != nil {
		_ = h.sessions.Stop(r.Context(), sess.ID)
	}
	if err := h.store.ArchiveTicket(r.Context(), id); err != nil {
		httpError(w, err, 500)
		return
	}
	h.bus.Publish(t.BoardID, "ticket_archived", t)
	board, _ := h.store.GetBoard(r.Context(), t.BoardID)
	if board != nil {
		h.hooks.Fire(&board.ID, hooks.EventTicketArchived, map[string]string{
			"ticket_id": fmt.Sprintf("%d", t.ID),
		})
	}
	w.WriteHeader(204)
}

func (h *handlers) unarchiveTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	if t.ArchivedAt == nil {
		httpError(w, fmt.Errorf("ticket is not archived"), 400)
		return
	}
	if err := h.store.UnarchiveTicket(r.Context(), id); err != nil {
		httpError(w, err, 500)
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
		httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, tickets)
}

func (h *handlers) deleteTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	if t.ArchivedAt == nil {
		httpError(w, fmt.Errorf("ticket must be archived before deletion"), 400)
		return
	}
	if sess, err := h.store.GetSessionByTicket(r.Context(), id); err == nil && sess != nil {
		_ = h.sessions.Destroy(r.Context(), sess.ID)
	}
	if err := h.store.DeleteTicket(r.Context(), id); err != nil {
		httpError(w, err, 500)
		return
	}
	h.bus.Publish(t.BoardID, "ticket_deleted", t)
	w.WriteHeader(204)
}

type syncTicketReq struct {
	Strategy string `json:"strategy"`
}

func (h *handlers) syncTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	var req syncTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if req.Strategy == "" {
		req.Strategy = "rebase"
	}
	switch req.Strategy {
	case "rebase", "merge":
	default:
		httpError(w, fmt.Errorf("strategy must be rebase or merge"), 400)
		return
	}
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	board, err := h.store.GetBoard(r.Context(), t.BoardID)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	if !loadSyncConfig(board.SourceRepoPath).allows(req.Strategy) {
		httpError(w, fmt.Errorf("strategy %s is disabled for this board", req.Strategy), 400)
		return
	}
	sess, err := h.store.GetSessionByTicket(r.Context(), id)
	if err != nil || sess == nil {
		httpError(w, fmt.Errorf("no session for ticket"), 404)
		return
	}
	if err := h.sessions.Sync(r.Context(), sess.ID, req.Strategy); err != nil {
		httpError(w, err, 409)
		return
	}
	h.bus.Publish(t.BoardID, "session_updated", sess)
	w.WriteHeader(204)
}

type mergeTicketReq struct {
	Strategy string `json:"strategy"`
}

func (h *handlers) mergeTicket(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	var req mergeTicketReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	switch req.Strategy {
	case "merge-commit", "squash", "rebase":
	default:
		httpError(w, fmt.Errorf("strategy must be merge-commit, squash, or rebase"), 400)
		return
	}
	t, err := h.store.GetTicket(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	board, err := h.store.GetBoard(r.Context(), t.BoardID)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	if !loadMergeConfig(board.SourceRepoPath).allows(req.Strategy) {
		httpError(w, fmt.Errorf("strategy %s is disabled for this board", req.Strategy), 400)
		return
	}
	sess, err := h.store.GetSessionByTicket(r.Context(), id)
	if err != nil || sess == nil {
		httpError(w, fmt.Errorf("no session for ticket"), 404)
		return
	}
	cols, err := h.store.ListColumns(r.Context(), t.BoardID)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	var doneCol *db.Column
	for i := range cols {
		if cols[i].Name == "Done" {
			doneCol = &cols[i]
			break
		}
	}
	if doneCol == nil {
		httpError(w, fmt.Errorf("board has no Done column"), 409)
		return
	}
	if err := h.sessions.Merge(r.Context(), sess.ID, req.Strategy); err != nil {
		httpError(w, err, 409)
		return
	}
	maxPos, err := h.store.MaxTicketPosition(r.Context(), doneCol.ID)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	if err := h.store.MoveTicket(r.Context(), t.ID, doneCol.ID, maxPos+1); err != nil {
		httpError(w, err, 500)
		return
	}
	if err := h.sessions.Destroy(r.Context(), sess.ID); err != nil {
		log.Printf("merge: destroy session %d: %v", sess.ID, err)
	}
	if updated, _ := h.store.GetTicket(r.Context(), id); updated != nil {
		h.bus.Publish(updated.BoardID, "ticket_moved", updated)
	}
	w.WriteHeader(204)
}

// Sessions

func (h *handlers) ensureSession(w http.ResponseWriter, r *http.Request) {
	ticketID := pathID(r, "id")
	t, err := h.store.GetTicket(r.Context(), ticketID)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	board, err := h.store.GetBoard(r.Context(), t.BoardID)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	sess, err := h.sessions.Ensure(r.Context(), board, t)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	h.bus.Publish(board.ID, "session_updated", sess)
	writeJSON(w, 201, sess)
}

func (h *handlers) startSession(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.sessions.Start(r.Context(), id)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	if t, _ := h.store.GetTicket(r.Context(), sess.TicketID); t != nil {
		h.bus.Publish(t.BoardID, "session_updated", sess)
	}
	writeJSON(w, 200, sess)
}

func (h *handlers) stopSession(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	if err := h.sessions.Stop(r.Context(), id); err != nil {
		httpError(w, err, 500)
		return
	}
	sess, _ := h.store.GetSession(r.Context(), id)
	if sess != nil {
		if t, _ := h.store.GetTicket(r.Context(), sess.TicketID); t != nil {
			h.bus.Publish(t.BoardID, "session_updated", sess)
		}
	}
	w.WriteHeader(204)
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
	var req updateSessionStatusReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("session %d status: decode body: %v (remote=%s)", id, err, r.RemoteAddr)
		httpError(w, err, 400)
		return
	}
	log.Printf("session %d status: received %q (remote=%s)", id, req.Status, r.RemoteAddr)
	var hookEvent string
	switch req.Status {
	case db.SessionStatusWorking:
		hookEvent = hooks.EventSessionWorking
	case db.SessionStatusIdle:
		hookEvent = hooks.EventSessionIdle
	case db.SessionStatusAwaitingPerm:
		hookEvent = hooks.EventSessionAwaitingPerm
	default:
		httpError(w, fmt.Errorf("status must be working, idle, or awaiting_perm"), 400)
		return
	}
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	if err := h.store.UpdateSessionStatus(r.Context(), id, req.Status); err != nil {
		httpError(w, err, 500)
		return
	}
	sess.Status = req.Status
	t, _ := h.store.GetTicket(r.Context(), sess.TicketID)
	if t != nil {
		h.bus.Publish(t.BoardID, "session_updated", sess)
		var boardID *int64
		if board, _ := h.store.GetBoard(r.Context(), t.BoardID); board != nil {
			boardID = &board.ID
		}
		h.hooks.Fire(boardID, hookEvent, map[string]string{
			"session_id": fmt.Sprintf("%d", sess.ID),
			"ticket_id":  fmt.Sprintf("%d", sess.TicketID),
		})
	}
	w.WriteHeader(204)
}

// Tasks

type taskInfo struct {
	tasks.VSCodeTask
	ContainerPort int  `json:"container_port,omitempty"`
	HasPort       bool `json:"has_port"`
}

func (h *handlers) discoverTasks(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	found, _ := tasks.Discover(sess.WorktreePath)
	out := make([]taskInfo, 0, len(found))
	for _, t := range found {
		info := taskInfo{VSCodeTask: t}
		if port, ok := tasks.PortFor(sess.WorktreePath, t.Label); ok {
			info.ContainerPort = port
			info.HasPort = true
		}
		out = append(out, info)
	}
	writeJSON(w, 200, out)
}

func (h *handlers) listTaskRuns(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	runs, err := h.store.ListTaskRuns(r.Context(), id)
	if err != nil {
		httpError(w, err, 500)
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
		httpError(w, err, 404)
		return
	}
	var req createTaskRunReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	found, err := tasks.Discover(sess.WorktreePath)
	if err != nil {
		httpError(w, err, 500)
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
		httpError(w, fmt.Errorf("task %q not found", req.Label), 404)
		return
	}
	tr, err := h.tasks.Start(r.Context(), sess, task)
	if err != nil {
		httpError(w, err, 500)
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
		httpError(w, err, 404)
		return
	}
	sess, err := h.store.GetSession(r.Context(), tr.SessionID)
	if err != nil {
		httpError(w, err, 500)
		return
	}
	if err := h.tasks.Stop(r.Context(), sess, tr); err != nil {
		httpError(w, err, 500)
		return
	}
	w.WriteHeader(204)
}

func (h *handlers) taskRunOutput(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, fmt.Errorf("streaming unsupported"), 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

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
		httpError(w, err, 500)
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
		httpError(w, err, 404)
		return
	}
	var req createPortReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if req.ContainerPort <= 0 {
		httpError(w, fmt.Errorf("container_port required"), 400)
		return
	}
	if err := h.ensurePortProxy(r.Context(), sess, req.Label, req.ContainerPort); err != nil {
		httpError(w, err, 500)
		return
	}
	ports, _ := h.store.ListPorts(r.Context(), id)
	writeJSON(w, 201, ports)
}

func (h *handlers) deletePort(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	ports, err := h.store.ListAllActivePorts(r.Context())
	if err != nil {
		httpError(w, err, 500)
		return
	}
	for _, p := range ports {
		if p.ID == id {
			h.sessions.Proxies().Close(p.HostPort)
			break
		}
	}
	if err := h.store.DeletePort(r.Context(), id); err != nil {
		httpError(w, err, 500)
		return
	}
	w.WriteHeader(204)
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
	t, err := h.store.GetTicket(ctx, sess.TicketID)
	if err != nil {
		return nil, err
	}
	return h.store.GetBoard(ctx, t.BoardID)
}

// PTY

func (h *handlers) wsPTY(w http.ResponseWriter, r *http.Request) {
	id := pathID(r, "id")
	sess, err := h.store.GetSession(r.Context(), id)
	if err != nil {
		httpError(w, err, 404)
		return
	}
	repoPath := ""
	if board, err := h.boardForSession(r.Context(), sess); err == nil && board != nil {
		repoPath = board.SourceRepoPath
	}
	resolved := harness.Resolve(repoPath)
	_ = h.sessions.AttachAgent(r.Context(), sess, w, r, resolved.PTYCommand, "/workspace")
}

// Settings — backed by the user-level config file at
// $XDG_CONFIG_HOME/kanban/config.toml. Empty harness == "no user override".

type settingsResp struct {
	Harness string `json:"harness"`
}

func (h *handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	id, _ := harness.ReadUserHarness()
	writeJSON(w, 200, settingsResp{Harness: id})
}

func (h *handlers) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Harness *string `json:"harness"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, err, 400)
		return
	}
	if req.Harness == nil {
		writeJSON(w, 200, settingsResp{Harness: firstNonEmpty(harness.ReadUserHarness)})
		return
	}
	id := *req.Harness
	if id != "" && !harness.IsKnown(id) {
		httpError(w, fmt.Errorf("unknown harness %q", id), 400)
		return
	}
	if err := harness.WriteUserHarness(id); err != nil {
		httpError(w, err, 500)
		return
	}
	writeJSON(w, 200, settingsResp{Harness: id})
}

func (h *handlers) listHarnesses(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, harness.Registry)
}

func firstNonEmpty(read func() (string, bool)) string {
	if v, ok := read(); ok {
		return v
	}
	return ""
}

// helpers

func pathID(r *http.Request, name string) int64 {
	v := r.PathValue(name)
	id, _ := strconv.ParseInt(v, 10, 64)
	return id
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, err error, code int) {
	log.Printf("http %d: %v", code, err)
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

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
	if out == "" {
		out = "x"
	}
	return out
}
