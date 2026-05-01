package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/git"
	"github.com/jmelahman/kanban/internal/harness"
	"github.com/jmelahman/kanban/internal/hooks"
)

type Manager struct {
	store  *db.Store
	docker *docker.Client
	hooks  *hooks.Runner

	proxies *docker.ProxyManager
	brokers *brokerSet
	apiBase string
}

func NewManager(store *db.Store, dc *docker.Client, h *hooks.Runner) *Manager {
	return &Manager{
		store:   store,
		docker:  dc,
		hooks:   h,
		proxies: docker.NewProxyManager(context.Background(), dc),
		brokers: newBrokerSet(dc),
	}
}

// SetAPIBase configures the URL session containers should use to call back
// into the kanban API (e.g. http://kanban:7474).
func (m *Manager) SetAPIBase(base string) { m.apiBase = base }

// Ensure creates a session row for a ticket if missing, allocating a worktree.
func (m *Manager) Ensure(ctx context.Context, board *db.Board, ticket *db.Ticket) (*db.Session, error) {
	if sess, err := m.store.GetSessionByTicket(ctx, ticket.ID); err == nil {
		if err := writeClaudeSettings(sess.WorktreePath); err != nil {
			log.Printf("write claude settings for ticket %d: %v", ticket.ID, err)
		}
		return sess, nil
	}

	branch := fmt.Sprintf("kanban/%s/%s", board.Slug, ticket.Slug)
	worktreePath := filepath.Join(board.WorktreeRoot, ticket.Slug)
	containerName := fmt.Sprintf("kanban-%s-%s", board.Slug, ticket.Slug)

	if _, statErr := os.Stat(worktreePath); statErr == nil {
		// Worktree directory already exists from a previous run; trust it.
	} else if err := git.AddWorktree(board.SourceRepoPath, branch, worktreePath, board.BaseBranch); err != nil {
		// Branch may already exist (orphaned). Try attaching it to a fresh worktree.
		if err2 := git.AddWorktreeFromExisting(board.SourceRepoPath, branch, worktreePath); err2 != nil {
			return nil, fmt.Errorf("create worktree: %w", err)
		}
	}

	sess := &db.Session{
		TicketID:      ticket.ID,
		WorktreePath:  worktreePath,
		BranchName:    branch,
		ContainerName: &containerName,
		Status:        db.SessionStatusStopped,
	}
	if err := m.store.UpsertSession(ctx, sess); err != nil {
		return nil, err
	}
	if err := writeClaudeSettings(worktreePath); err != nil {
		log.Printf("write claude settings for ticket %d: %v", ticket.ID, err)
	}
	return sess, nil
}

// Start brings up the devcontainer for a session.
func (m *Manager) Start(ctx context.Context, sessionID int64) (*db.Session, error) {
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	switch sess.Status {
	case db.SessionStatusStopped, db.SessionStatusError:
		// proceed
	default:
		return sess, nil
	}

	// Stale container from a prior run (e.g. host reboot): clear the reference
	// so we don't try to reuse a vanished container ID below.
	if sess.ContainerID != nil && *sess.ContainerID != "" {
		cleared := ""
		sess.ContainerID = &cleared
	}

	cfg, err := docker.LoadDevcontainer(sess.WorktreePath)
	if err != nil {
		_ = m.store.UpdateSessionStatus(ctx, sess.ID, db.SessionStatusError)
		return nil, err
	}

	_ = m.store.UpdateSessionStatus(ctx, sess.ID, db.SessionStatusStarting)

	ports, _ := m.store.ListPorts(ctx, sess.ID)
	mappings := make([]docker.PortMapping, 0, len(ports))
	for _, p := range ports {
		mappings = append(mappings, docker.PortMapping{HostPort: p.HostPort, ContainerPort: p.ContainerPort})
	}

	containerName := ""
	if sess.ContainerName != nil {
		containerName = *sess.ContainerName
	}

	// Remove any pre-existing container with this name (e.g. left over after a
	// host reboot). Docker would otherwise reject ContainerCreate with a name
	// conflict.
	if containerName != "" {
		_ = m.docker.RemoveContainer(ctx, containerName)
	}

	board, _ := m.boardForSession(ctx, sess)
	sourceRepoPath := ""
	if board != nil {
		sourceRepoPath = board.SourceRepoPath
	}

	res, err := m.docker.Spawn(ctx, cfg, docker.SpawnOptions{
		WorktreePath:   sess.WorktreePath,
		SourceRepoPath: sourceRepoPath,
		ContainerName:  containerName,
		Ports:          mappings,
		ExtraEnv: map[string]string{
			"KANBAN_SESSION_ID": fmt.Sprintf("%d", sess.ID),
			"KANBAN_API_URL":    m.apiBase,
		},
		AttachNetwork: docker.KanbanNetworkName,
	})
	if err != nil {
		_ = m.store.UpdateSessionStatus(ctx, sess.ID, db.SessionStatusError)
		return nil, err
	}

	now := time.Now().Unix()
	sess.ContainerID = &res.ContainerID
	sess.Status = db.SessionStatusIdle
	sess.StartedAt = &now
	sess.StoppedAt = nil
	if err := m.store.UpsertSession(ctx, sess); err != nil {
		return nil, err
	}

	var boardID *int64
	if board != nil {
		boardID = &board.ID
	}
	m.hooks.Fire(boardID, hooks.EventSessionStarted, map[string]string{
		"session_id": fmt.Sprintf("%d", sess.ID),
		"ticket_id":  fmt.Sprintf("%d", sess.TicketID),
	})

	return sess, nil
}

// Stop tears down the devcontainer; worktree is preserved.
func (m *Manager) Stop(ctx context.Context, sessionID int64) error {
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	// Tear down any persistent PTY broker so its hijacked exec connection is
	// closed before we kill the container underneath it.
	m.brokers.closeFor(sessionID)
	if sess.ContainerID != nil && *sess.ContainerID != "" {
		_ = m.docker.StopContainer(ctx, *sess.ContainerID, 10*time.Second)
		_ = m.docker.RemoveContainer(ctx, *sess.ContainerID)
	}
	now := time.Now().Unix()
	sess.Status = db.SessionStatusStopped
	sess.StoppedAt = &now
	cleared := ""
	sess.ContainerID = &cleared
	if err := m.store.UpsertSession(ctx, sess); err != nil {
		return err
	}

	// Close any active proxies for this session.
	ports, _ := m.store.ListPorts(ctx, sess.ID)
	for _, p := range ports {
		if p.ProxyActive {
			m.proxies.Close(p.HostPort)
			_ = m.store.SetPortActive(ctx, p.ID, false)
		}
	}

	board, _ := m.boardForSession(ctx, sess)
	var boardID *int64
	if board != nil {
		boardID = &board.ID
	}
	m.hooks.Fire(boardID, hooks.EventSessionStopped, map[string]string{
		"session_id": fmt.Sprintf("%d", sess.ID),
	})
	return nil
}

// Destroy fully tears down a session: stops the container, removes the
// worktree directory, deletes the branch, and removes the session row.
// Errors from filesystem/git cleanup are non-fatal and reported via the
// returned error only when the DB row removal itself fails.
func (m *Manager) Destroy(ctx context.Context, sessionID int64) error {
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	_ = m.Stop(ctx, sessionID)

	board, _ := m.boardForSession(ctx, sess)
	if board != nil && sess.WorktreePath != "" {
		_ = git.RemoveWorktree(board.SourceRepoPath, sess.WorktreePath)
	}
	if sess.WorktreePath != "" {
		_ = os.RemoveAll(sess.WorktreePath)
	}
	if board != nil && sess.BranchName != "" {
		_ = git.DeleteBranch(board.SourceRepoPath, sess.BranchName)
	}
	return m.store.DeleteSession(ctx, sess.ID)
}

// Sync brings the session's branch up to date with the board's base branch
// using either "rebase" or "merge". Aborts on conflict and surfaces the error.
func (m *Manager) Sync(ctx context.Context, sessionID int64, strategy string) error {
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.WorktreePath == "" {
		return fmt.Errorf("session has no worktree")
	}
	board, err := m.boardForSession(ctx, sess)
	if err != nil {
		return err
	}
	clean, err := git.IsClean(sess.WorktreePath)
	if err != nil {
		return fmt.Errorf("check worktree clean: %w", err)
	}
	if !clean {
		return fmt.Errorf("worktree has uncommitted changes; commit or stash before syncing")
	}
	switch strategy {
	case "rebase":
		if err := git.Rebase(sess.WorktreePath, board.BaseBranch); err != nil {
			git.RebaseAbort(sess.WorktreePath)
			return fmt.Errorf("rebase aborted: %w", err)
		}
	case "merge":
		if err := git.Merge(sess.WorktreePath, board.BaseBranch); err != nil {
			git.MergeAbort(sess.WorktreePath)
			return fmt.Errorf("merge aborted: %w", err)
		}
	default:
		return fmt.Errorf("unknown strategy %q (want rebase or merge)", strategy)
	}
	return nil
}

// Merge integrates the session's branch into the board's base branch in the
// source repo. The source repo must be clean and have base_branch checked out.
// On any git failure the source repo and worktree are restored to their
// pre-merge state. Strategy is one of "merge-commit", "squash", "rebase".
func (m *Manager) Merge(ctx context.Context, sessionID int64, strategy string) error {
	sess, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.WorktreePath == "" || sess.BranchName == "" {
		return fmt.Errorf("session has no worktree")
	}
	board, err := m.boardForSession(ctx, sess)
	if err != nil {
		return err
	}
	ticket, err := m.store.GetTicket(ctx, sess.TicketID)
	if err != nil {
		return err
	}

	if clean, err := git.IsClean(sess.WorktreePath); err != nil {
		return fmt.Errorf("check worktree clean: %w", err)
	} else if !clean {
		if err := git.AddAll(sess.WorktreePath); err != nil {
			return fmt.Errorf("stage pending changes: %w", err)
		}
		msg := ticket.Title
		h, _ := harness.Resolve(board.SourceRepoPath)
		if generated, err := m.generateCommitMessage(ctx, sess, h, ticket.Title); err == nil {
			msg = generated
		} else {
			log.Printf("merge: ai commit message unavailable, using ticket title: %v", err)
		}
		if err := git.Commit(sess.WorktreePath, msg); err != nil {
			return fmt.Errorf("commit pending changes: %w", err)
		}
	}
	if clean, err := git.IsClean(board.SourceRepoPath); err != nil {
		return fmt.Errorf("check source repo clean: %w", err)
	} else if !clean {
		return fmt.Errorf("source repo has uncommitted changes; commit or stash before merging")
	}
	cur, err := git.CurrentBranch(board.SourceRepoPath)
	if err != nil {
		return fmt.Errorf("read source repo branch: %w", err)
	}
	if cur != board.BaseBranch {
		return fmt.Errorf("source repo must have %s checked out (currently on %q)", board.BaseBranch, cur)
	}
	baseHead, err := git.CurrentHead(board.SourceRepoPath, "HEAD")
	if err != nil {
		return fmt.Errorf("read base head: %w", err)
	}

	switch strategy {
	case "merge-commit":
		if err := git.MergeNoFF(board.SourceRepoPath, sess.BranchName); err != nil {
			git.MergeAbort(board.SourceRepoPath)
			return fmt.Errorf("merge aborted: %w", err)
		}
	case "squash":
		msg := fmt.Sprintf("%s (#%d)", ticket.Title, ticket.ID)
		if err := git.MergeSquash(board.SourceRepoPath, sess.BranchName, msg); err != nil {
			git.MergeAbort(board.SourceRepoPath)
			git.ResetHard(board.SourceRepoPath, baseHead)
			return fmt.Errorf("squash aborted: %w", err)
		}
	case "rebase":
		if err := git.Rebase(sess.WorktreePath, board.BaseBranch); err != nil {
			git.RebaseAbort(sess.WorktreePath)
			return fmt.Errorf("rebase aborted: %w", err)
		}
		if err := git.MergeFFOnly(board.SourceRepoPath, sess.BranchName); err != nil {
			return fmt.Errorf("fast-forward aborted: %w", err)
		}
	default:
		return fmt.Errorf("unknown strategy %q (want merge-commit, squash, or rebase)", strategy)
	}
	return nil
}

// generateCommitMessage renders the harness's commit-message script, runs it
// inside the session's container with the staged diff piped via stdin, and
// returns the trimmed first line of stdout. Returns an error (so the caller
// can fall back to the ticket title) when the container is not running, the
// harness has no template, or the script fails.
func (m *Manager) generateCommitMessage(ctx context.Context, sess *db.Session, h harness.Harness, ticketTitle string) (string, error) {
	if sess.ContainerID == nil || *sess.ContainerID == "" {
		return "", fmt.Errorf("container not running")
	}
	prompt := fmt.Sprintf(
		"Write a one-line git commit message in imperative mood for the staged diff piped via stdin. The change is for the ticket %q. Output only the commit message text - no preamble, no quotes, no markdown, no code fences.",
		ticketTitle,
	)
	script, err := h.RenderCommitScript(prompt)
	if err != nil {
		return "", err
	}
	if script == "" {
		return "", fmt.Errorf("harness %q has no commit-message template", h.ID)
	}
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	out, err := m.docker.ExecRun(cctx, *sess.ContainerID, []string{"sh", "-lc", script})
	if err != nil {
		return "", err
	}
	msg := strings.TrimSpace(out)
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	msg = strings.Trim(msg, "\"' \t")
	if msg == "" {
		return "", fmt.Errorf("empty message")
	}
	return msg, nil
}

func (m *Manager) Proxies() *docker.ProxyManager { return m.proxies }

func (m *Manager) Docker() *docker.Client { return m.docker }

func (m *Manager) boardForSession(ctx context.Context, sess *db.Session) (*db.Board, error) {
	t, err := m.store.GetTicket(ctx, sess.TicketID)
	if err != nil {
		return nil, err
	}
	return m.store.GetBoard(ctx, t.BoardID)
}
