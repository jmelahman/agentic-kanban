package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("not found")

// Boards

func (s *Store) CreateBoard(ctx context.Context, b *Board) error {
	if err := s.CreateBoardRaw(ctx, b); err != nil {
		return err
	}
	return s.createDefaultColumns(ctx, b.ID)
}

// CreateBoardRaw inserts a board row without seeding the default
// Backlog/In Progress/Review/Done columns. Callers are responsible for
// installing whatever column layout they need afterwards. Used by
// errreport to bootstrap an Errors board with its own three-column flow.
func (s *Store) CreateBoardRaw(ctx context.Context, b *Board) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO boards (name, slug, repo_path, mount_path, worktree_root, base_branch, branch_prefix, git_author_name, git_author_email) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.Name, b.Slug, nullIfEmpty(b.RepoPath), nullIfEmpty(b.MountPath), nullIfEmpty(b.WorktreeRoot), b.BaseBranch, nullIfEmpty(b.BranchPrefix), nullIfEmpty(b.GitAuthorName), nullIfEmpty(b.GitAuthorEmail),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	b.ID = id
	return nil
}

// CreateColumn inserts a single column at the given position. Used by
// errreport to install its custom New/Investigating/Resolved layout.
func (s *Store) CreateColumn(ctx context.Context, c *Column) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO columns (board_id, name, position) VALUES (?, ?, ?)`,
		c.BoardID, c.Name, c.Position,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

func (s *Store) createDefaultColumns(ctx context.Context, boardID int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO columns (board_id, name, position) VALUES (?,?,?),(?,?,?),(?,?,?),(?,?,?)`,
		boardID, "Backlog", 0,
		boardID, "In Progress", 1,
		boardID, "Review", 2,
		boardID, "Done", 3,
	)
	return err
}

func (s *Store) ListBoards(ctx context.Context) ([]Board, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, slug, repo_path, mount_path, worktree_root, base_branch, branch_prefix, git_author_name, git_author_email, created_at FROM boards ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	boards := []Board{}
	for rows.Next() {
		b, err := scanBoard(rows)
		if err != nil {
			return nil, err
		}
		boards = append(boards, *b)
	}
	return boards, rows.Err()
}

func (s *Store) UpdateBoard(ctx context.Context, b *Board) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE boards SET name=?, repo_path=?, mount_path=?, worktree_root=?, base_branch=?, branch_prefix=?, git_author_name=?, git_author_email=? WHERE id=?`,
		b.Name, nullIfEmpty(b.RepoPath), nullIfEmpty(b.MountPath), nullIfEmpty(b.WorktreeRoot), b.BaseBranch, nullIfEmpty(b.BranchPrefix), nullIfEmpty(b.GitAuthorName), nullIfEmpty(b.GitAuthorEmail), b.ID,
	)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

func (s *Store) DeleteBoard(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM boards WHERE id=?`, id)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

func (s *Store) GetBoard(ctx context.Context, id int64) (*Board, error) {
	b, err := scanBoard(s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, repo_path, mount_path, worktree_root, base_branch, branch_prefix, git_author_name, git_author_email, created_at FROM boards WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

func (s *Store) GetBoardBySlug(ctx context.Context, slug string) (*Board, error) {
	b, err := scanBoard(s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, repo_path, mount_path, worktree_root, base_branch, branch_prefix, git_author_name, git_author_email, created_at FROM boards WHERE slug=?`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

// Columns

func (s *Store) ListColumns(ctx context.Context, boardID int64) ([]Column, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, board_id, name, position FROM columns WHERE board_id=? ORDER BY position`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := []Column{}
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.ID, &c.BoardID, &c.Name, &c.Position); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func (s *Store) FindColumnByName(ctx context.Context, boardID int64, name string) (*Column, error) {
	var c Column
	err := s.db.QueryRowContext(ctx,
		`SELECT id, board_id, name, position FROM columns WHERE board_id=? AND name=? LIMIT 1`,
		boardID, name,
	).Scan(&c.ID, &c.BoardID, &c.Name, &c.Position)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Tickets

func (s *Store) CreateTicket(ctx context.Context, t *Ticket) error {
	var maxPos sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MAX(position) FROM tickets WHERE column_id=? AND archived_at IS NULL`, t.ColumnID,
	).Scan(&maxPos); err != nil {
		return err
	}
	t.Position = int(maxPos.Int64) + 1

	baseSlug := t.Slug
	for attempt := 1; attempt <= 100; attempt++ {
		if attempt > 1 {
			t.Slug = fmt.Sprintf("%s-%d", baseSlug, attempt)
		}
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO tickets (board_id, column_id, title, slug, body, position, fingerprint) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			t.BoardID, t.ColumnID, t.Title, t.Slug, t.Body, t.Position, nullIfEmpty(t.Fingerprint),
		)
		if err != nil {
			if strings.Contains(err.Error(), "tickets.board_id, tickets.slug") {
				continue
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		t.ID = id
		return nil
	}
	return fmt.Errorf("could not allocate unique slug for %q after 100 attempts", baseSlug)
}

func (s *Store) ListTickets(ctx context.Context, boardID int64) ([]Ticket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, board_id, column_id, title, slug, body, position, created_at, archived_at
         FROM tickets WHERE board_id=? AND archived_at IS NULL ORDER BY column_id, position`,
		boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tickets := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, *t)
	}
	return tickets, rows.Err()
}

func (s *Store) GetTicket(ctx context.Context, id int64) (*Ticket, error) {
	t, err := scanTicket(s.db.QueryRowContext(ctx,
		`SELECT id, board_id, column_id, title, slug, body, position, created_at, archived_at FROM tickets WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// FindOpenTicketByFingerprint returns the open (non-archived) ticket on
// boardID whose fingerprint matches, or ErrNotFound if none exists. Used by
// errreport for dedup: a recurring error updates the existing ticket rather
// than filing a new one.
func (s *Store) FindOpenTicketByFingerprint(ctx context.Context, boardID int64, fingerprint string) (*Ticket, error) {
	if fingerprint == "" {
		return nil, ErrNotFound
	}
	var t Ticket
	var fp sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, board_id, column_id, title, slug, body, position, created_at, archived_at, fingerprint
		 FROM tickets
		 WHERE board_id=? AND fingerprint=? AND archived_at IS NULL
		 LIMIT 1`,
		boardID, fingerprint,
	).Scan(&t.ID, &t.BoardID, &t.ColumnID, &t.Title, &t.Slug, &t.Body, &t.Position, &t.CreatedAt, &t.ArchivedAt, &fp)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	t.Fingerprint = fp.String
	return &t, nil
}

func (s *Store) UpdateTicket(ctx context.Context, t *Ticket) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET title=?, body=? WHERE id=?`,
		t.Title, t.Body, t.ID,
	)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

func (s *Store) MoveTicket(ctx context.Context, ticketID, columnID int64, position int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var oldCol int64
	var oldPos int
	if err := tx.QueryRowContext(ctx,
		`SELECT column_id, position FROM tickets WHERE id=?`, ticketID,
	).Scan(&oldCol, &oldPos); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	if oldCol == columnID {
		if oldPos == position {
			return tx.Commit()
		}
		if oldPos < position {
			if _, err := tx.ExecContext(ctx,
				`UPDATE tickets SET position = position - 1
				 WHERE column_id=? AND id != ? AND position > ? AND position <= ?`,
				columnID, ticketID, oldPos, position); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE tickets SET position = position + 1
				 WHERE column_id=? AND id != ? AND position >= ? AND position < ?`,
				columnID, ticketID, position, oldPos); err != nil {
				return err
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE tickets SET position = position - 1 WHERE column_id=? AND position > ?`,
			oldCol, oldPos); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE tickets SET position = position + 1 WHERE column_id=? AND position >= ?`,
			columnID, position); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE tickets SET column_id=?, position=? WHERE id=?`,
		columnID, position, ticketID); err != nil {
		return err
	}
	return tx.Commit()
}

// MaxTicketPosition returns the largest position among non-archived tickets in
// columnID, or 0 if the column is empty.
func (s *Store) MaxTicketPosition(ctx context.Context, columnID int64) (int, error) {
	var maxPos sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(position) FROM tickets WHERE column_id=? AND archived_at IS NULL`, columnID,
	).Scan(&maxPos)
	if err != nil {
		return 0, err
	}
	return int(maxPos.Int64), nil
}

func (s *Store) ArchiveTicket(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tickets SET archived_at=unixepoch() WHERE id=?`, id)
	return err
}

// ListTicketsInColumn returns non-archived tickets in a column ordered by position.
func (s *Store) ListTicketsInColumn(ctx context.Context, columnID int64) ([]Ticket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, board_id, column_id, title, slug, body, position, created_at, archived_at
         FROM tickets WHERE column_id=? AND archived_at IS NULL ORDER BY position`,
		columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tickets := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, *t)
	}
	return tickets, rows.Err()
}

// ArchiveTicketsInColumn archives every non-archived ticket in the column in
// one statement. Returns the number of tickets affected.
func (s *Store) ArchiveTicketsInColumn(ctx context.Context, columnID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE tickets SET archived_at=unixepoch() WHERE column_id=? AND archived_at IS NULL`, columnID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) UnarchiveTicket(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var columnID int64
	if err := tx.QueryRowContext(ctx, `SELECT column_id FROM tickets WHERE id=?`, id).Scan(&columnID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var maxPos sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT MAX(position) FROM tickets WHERE column_id=? AND archived_at IS NULL`, columnID,
	).Scan(&maxPos); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE tickets SET archived_at=NULL, position=? WHERE id=?`, int(maxPos.Int64)+1, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListArchivedTickets(ctx context.Context, boardID int64) ([]Ticket, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, board_id, column_id, title, slug, body, position, created_at, archived_at
         FROM tickets WHERE board_id=? AND archived_at IS NOT NULL ORDER BY archived_at DESC`,
		boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tickets := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, *t)
	}
	return tickets, rows.Err()
}

func (s *Store) DeleteTicket(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tickets WHERE id=?`, id)
	return err
}

// DeleteAllArchivedTickets deletes every archived ticket on the board in one
// statement. Returns the number of tickets affected.
func (s *Store) DeleteAllArchivedTickets(ctx context.Context, boardID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM tickets WHERE board_id=? AND archived_at IS NOT NULL`, boardID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Sessions

func (s *Store) UpsertSession(ctx context.Context, sess *Session) error {
	if sess.ID == 0 {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO sessions (ticket_id, worktree_path, branch_name, container_id, container_name, status, started_at, stopped_at, pr_state, pr_number, pr_url, pr_title, mount_path, repo_path, claude_session_id)
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sess.TicketID, sess.WorktreePath, sess.BranchName, sess.ContainerID, sess.ContainerName, sess.Status, sess.StartedAt, sess.StoppedAt, sess.PRState, sess.PRNumber, nullIfEmpty(sess.PRURL), nullIfEmpty(sess.PRTitle), nullIfEmpty(sess.MountPath), nullIfEmpty(sess.RepoPath), nullIfEmpty(sess.ClaudeSessionID),
		)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		sess.ID = id
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET worktree_path=?, branch_name=?, container_id=?, container_name=?, status=?, started_at=?, stopped_at=?, pr_state=?, pr_number=?, pr_url=?, pr_title=?, mount_path=?, repo_path=?, claude_session_id=? WHERE id=?`,
		sess.WorktreePath, sess.BranchName, sess.ContainerID, sess.ContainerName, sess.Status, sess.StartedAt, sess.StoppedAt, sess.PRState, sess.PRNumber, nullIfEmpty(sess.PRURL), nullIfEmpty(sess.PRTitle), nullIfEmpty(sess.MountPath), nullIfEmpty(sess.RepoPath), nullIfEmpty(sess.ClaudeSessionID), sess.ID,
	)
	return err
}

func (s *Store) UpdateSessionStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET status=? WHERE id=?`, status, id)
	return err
}

// UpdateClaudeSessionID records the Claude Code session UUID for the given
// Kanban session. Called by the SessionStart hook running inside the session
// container so subsequent `claude` launches can `--resume <uuid>`. Passing an
// empty uuid clears the stored value (treated as SQL NULL).
func (s *Store) UpdateClaudeSessionID(ctx context.Context, id int64, uuid string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET claude_session_id=? WHERE id=?`,
		nullIfEmpty(uuid), id,
	)
	if err != nil {
		return err
	}
	return affectedOrNotFound(res)
}

func (s *Store) UpdateSessionPR(ctx context.Context, id int64, prState string, prNumber *int64, prURL, prTitle string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET pr_state=?, pr_number=?, pr_url=?, pr_title=? WHERE id=?`,
		prState, prNumber, nullIfEmpty(prURL), nullIfEmpty(prTitle), id,
	)
	return err
}

func (s *Store) GetSession(ctx context.Context, id int64) (*Session, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT id, ticket_id, worktree_path, branch_name, container_id, container_name, status, started_at, stopped_at, pr_state, pr_number, pr_url, pr_title, mount_path, repo_path, claude_session_id FROM sessions WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sess, err
}

func (s *Store) GetSessionByTicket(ctx context.Context, ticketID int64) (*Session, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT id, ticket_id, worktree_path, branch_name, container_id, container_name, status, started_at, stopped_at, pr_state, pr_number, pr_url, pr_title, mount_path, repo_path, claude_session_id FROM sessions WHERE ticket_id=?`, ticketID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return sess, err
}

func (s *Store) ListSessionsByBoard(ctx context.Context, boardID int64) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id, s.ticket_id, s.worktree_path, s.branch_name, s.container_id, s.container_name, s.status, s.started_at, s.stopped_at, s.pr_state, s.pr_number, s.pr_url, s.pr_title, s.mount_path, s.repo_path, s.claude_session_id
         FROM sessions s JOIN tickets t ON t.id=s.ticket_id WHERE t.board_id=?`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := []Session{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *sess)
	}
	return sessions, rows.Err()
}

func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id)
	return err
}

// Port allocations

func (s *Store) CreatePort(ctx context.Context, p *PortAllocation) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO port_allocations (session_id, label, container_port, host_port, proxy_active) VALUES (?, ?, ?, ?, ?)`,
		p.SessionID, p.Label, p.ContainerPort, p.HostPort, boolToInt(p.ProxyActive),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = id
	return nil
}

func (s *Store) ListPorts(ctx context.Context, sessionID int64) ([]PortAllocation, error) {
	return s.queryPorts(ctx,
		`SELECT id, session_id, label, container_port, host_port, proxy_active FROM port_allocations WHERE session_id=?`,
		sessionID)
}

func (s *Store) ListAllActivePorts(ctx context.Context) ([]PortAllocation, error) {
	return s.queryPorts(ctx,
		`SELECT id, session_id, label, container_port, host_port, proxy_active FROM port_allocations WHERE proxy_active=1`)
}

func (s *Store) queryPorts(ctx context.Context, query string, args ...any) ([]PortAllocation, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ports := []PortAllocation{}
	for rows.Next() {
		p, err := scanPort(rows)
		if err != nil {
			return nil, err
		}
		ports = append(ports, *p)
	}
	return ports, rows.Err()
}

func (s *Store) GetPort(ctx context.Context, id int64) (*PortAllocation, error) {
	p, err := scanPort(s.db.QueryRowContext(ctx,
		`SELECT id, session_id, label, container_port, host_port, proxy_active FROM port_allocations WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) SetPortActive(ctx context.Context, id int64, active bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE port_allocations SET proxy_active=? WHERE id=?`, boolToInt(active), id)
	return err
}

func (s *Store) DeletePort(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM port_allocations WHERE id=?`, id)
	return err
}

func (s *Store) AllocateHostPort(ctx context.Context, start, end int) (int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT host_port FROM port_allocations WHERE host_port BETWEEN ? AND ?`,
		start, end)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	taken := map[int]bool{}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return 0, err
		}
		taken[p] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for p := start; p <= end; p++ {
		if !taken[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port in range %d-%d", start, end)
}

// Task runs

func (s *Store) CreateTaskRun(ctx context.Context, tr *TaskRun) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO task_runs (session_id, task_label, command, exec_id, status) VALUES (?, ?, ?, ?, ?)`,
		tr.SessionID, tr.TaskLabel, tr.Command, tr.ExecID, tr.Status,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	tr.ID = id
	return nil
}

func (s *Store) UpdateTaskRunStatus(ctx context.Context, id int64, status string, exitCode *int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE task_runs SET status=?, exit_code=?, stopped_at=unixepoch() WHERE id=?`, status, exitCode, id)
	return err
}

func (s *Store) GetTaskRun(ctx context.Context, id int64) (*TaskRun, error) {
	tr, err := scanTaskRun(s.db.QueryRowContext(ctx,
		`SELECT id, session_id, task_label, command, exec_id, status, exit_code, started_at, stopped_at FROM task_runs WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return tr, err
}

func (s *Store) ListTaskRuns(ctx context.Context, sessionID int64) ([]TaskRun, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, task_label, command, exec_id, status, exit_code, started_at, stopped_at FROM task_runs WHERE session_id=? ORDER BY id DESC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []TaskRun{}
	for rows.Next() {
		tr, err := scanTaskRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *tr)
	}
	return runs, rows.Err()
}

// Hook configs

func (s *Store) ListHooks(ctx context.Context, boardID *int64, event string) ([]HookConfig, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, board_id, event, command, enabled FROM hook_configs
         WHERE enabled=1 AND event=? AND (board_id IS NULL OR board_id=?)`,
		event, boardID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hooks := []HookConfig{}
	for rows.Next() {
		var h HookConfig
		var enabled int
		if err := rows.Scan(&h.ID, &h.BoardID, &h.Event, &h.Command, &enabled); err != nil {
			return nil, err
		}
		h.Enabled = enabled != 0
		hooks = append(hooks, h)
	}
	return hooks, rows.Err()
}

// scanner is the common surface of *sql.Row and *sql.Rows, letting the
// scanXxx helpers below serve both single-row and rows-loop callers.
type scanner interface {
	Scan(dest ...any) error
}

func affectedOrNotFound(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanBoard(sc scanner) (*Board, error) {
	var b Board
	var repo, mount, worktreeRoot, branchPrefix, gitAuthorName, gitAuthorEmail sql.NullString
	if err := sc.Scan(&b.ID, &b.Name, &b.Slug, &repo, &mount, &worktreeRoot, &b.BaseBranch, &branchPrefix, &gitAuthorName, &gitAuthorEmail, &b.CreatedAt); err != nil {
		return nil, err
	}
	b.RepoPath = repo.String
	b.MountPath = mount.String
	b.WorktreeRoot = worktreeRoot.String
	b.BranchPrefix = branchPrefix.String
	b.GitAuthorName = gitAuthorName.String
	b.GitAuthorEmail = gitAuthorEmail.String
	return &b, nil
}

func scanSession(sc scanner) (*Session, error) {
	var sess Session
	var mount, repo, prURL, prTitle, claudeSessionID sql.NullString
	if err := sc.Scan(&sess.ID, &sess.TicketID, &sess.WorktreePath, &sess.BranchName, &sess.ContainerID, &sess.ContainerName, &sess.Status, &sess.StartedAt, &sess.StoppedAt, &sess.PRState, &sess.PRNumber, &prURL, &prTitle, &mount, &repo, &claudeSessionID); err != nil {
		return nil, err
	}
	sess.PRURL = prURL.String
	sess.PRTitle = prTitle.String
	sess.MountPath = mount.String
	sess.RepoPath = repo.String
	sess.ClaudeSessionID = claudeSessionID.String
	return &sess, nil
}

func scanTicket(sc scanner) (*Ticket, error) {
	var t Ticket
	if err := sc.Scan(&t.ID, &t.BoardID, &t.ColumnID, &t.Title, &t.Slug, &t.Body, &t.Position, &t.CreatedAt, &t.ArchivedAt); err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTaskRun(sc scanner) (*TaskRun, error) {
	var tr TaskRun
	if err := sc.Scan(&tr.ID, &tr.SessionID, &tr.TaskLabel, &tr.Command, &tr.ExecID, &tr.Status, &tr.ExitCode, &tr.StartedAt, &tr.StoppedAt); err != nil {
		return nil, err
	}
	return &tr, nil
}

func scanPort(sc scanner) (*PortAllocation, error) {
	var p PortAllocation
	var active int
	if err := sc.Scan(&p.ID, &p.SessionID, &p.Label, &p.ContainerPort, &p.HostPort, &active); err != nil {
		return nil, err
	}
	p.ProxyActive = active != 0
	return &p, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullIfEmpty returns sql.NullString{Valid:false} when s is empty, so optional
// columns are stored as SQL NULL (matching the schema's nullable definitions)
// rather than as empty strings.
func nullIfEmpty(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
