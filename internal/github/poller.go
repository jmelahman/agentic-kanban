// Package github polls `gh pr list` per board and moves tickets between
// columns according to the PR's state, mirroring how the session manager
// reflects Claude session state on a ticket.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/jmelahman/kanban/internal/db"
)

const (
	PRStateDraft  = "draft"
	PRStateOpen   = "open"
	PRStateMerged = "merged"
	PRStateClosed = "closed"
)

// Config captures the [github] section of a board's .kanban.toml.
type Config struct {
	AutoMove     bool
	DraftColumn  string
	ReviewColumn string
	DoneColumn   string
	ClosedColumn string
}

func defaultConfig() Config {
	return Config{
		AutoMove:     false,
		DraftColumn:  "In Progress",
		ReviewColumn: "Review",
		DoneColumn:   "Done",
		ClosedColumn: "",
	}
}

type tomlFile struct {
	GitHub *struct {
		AutoMove     *bool   `toml:"auto_move"`
		DraftColumn  *string `toml:"draft_column"`
		ReviewColumn *string `toml:"review_column"`
		DoneColumn   *string `toml:"done_column"`
		ClosedColumn *string `toml:"closed_column"`
	} `toml:"github"`
}

// LoadConfig reads <repoPath>/.kanban.toml and merges its [github] section
// onto defaults. Missing or unparseable file yields disabled defaults.
func LoadConfig(repoPath string) Config {
	cfg := defaultConfig()
	if repoPath == "" {
		return cfg
	}
	data, err := os.ReadFile(filepath.Join(repoPath, ".kanban.toml"))
	if err != nil {
		return cfg
	}
	var f tomlFile
	if err := toml.Unmarshal(data, &f); err != nil {
		return cfg
	}
	if f.GitHub == nil {
		return cfg
	}
	if f.GitHub.AutoMove != nil {
		cfg.AutoMove = *f.GitHub.AutoMove
	}
	if f.GitHub.DraftColumn != nil {
		cfg.DraftColumn = *f.GitHub.DraftColumn
	}
	if f.GitHub.ReviewColumn != nil {
		cfg.ReviewColumn = *f.GitHub.ReviewColumn
	}
	if f.GitHub.DoneColumn != nil {
		cfg.DoneColumn = *f.GitHub.DoneColumn
	}
	if f.GitHub.ClosedColumn != nil {
		cfg.ClosedColumn = *f.GitHub.ClosedColumn
	}
	return cfg
}

// Publisher publishes board events. *api.EventBus satisfies this.
type Publisher interface {
	Publish(boardID int64, typ string, data any)
}

// SessionDestroyer is the subset of *session.Manager the poller needs to
// clean up a worktree + container after a PR is merged externally.
type SessionDestroyer interface {
	Destroy(ctx context.Context, sessionID int64) error
}

type Poller struct {
	store    *db.Store
	bus      Publisher
	sessions SessionDestroyer
	interval time.Duration
}

// NewPoller constructs a poller. interval is the global tick rate.
func NewPoller(store *db.Store, bus Publisher, sessions SessionDestroyer, interval time.Duration) *Poller {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Poller{store: store, bus: bus, sessions: sessions, interval: interval}
}

// Start blocks until ctx is done, ticking once per interval. If `gh` isn't
// on PATH, logs once and returns without polling.
func (p *Poller) Start(ctx context.Context) {
	if _, err := exec.LookPath("gh"); err != nil {
		log.Printf("github poller: gh CLI not found; auto-move disabled")
		return
	}
	t := time.NewTicker(p.interval)
	defer t.Stop()
	p.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.tick(ctx)
		}
	}
}

func (p *Poller) tick(ctx context.Context) {
	boards, err := p.store.ListBoards(ctx)
	if err != nil {
		log.Printf("github poller: list boards: %v", err)
		return
	}
	for i := range boards {
		b := &boards[i]
		cfg := LoadConfig(b.SourceRepoPath)
		if !cfg.AutoMove {
			continue
		}
		if err := p.syncBoard(ctx, b, cfg); err != nil {
			log.Printf("github poller: board %d: %v", b.ID, err)
		}
	}
}

type ghPR struct {
	HeadRefName string `json:"headRefName"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
}

func (p *Poller) syncBoard(ctx context.Context, board *db.Board, cfg Config) error {
	sessions, err := p.store.ListSessionsByBoard(ctx, board.ID)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	branches := make(map[string]struct{}, len(sessions))
	for i := range sessions {
		if b := sessions[i].BranchName; b != "" {
			branches[b] = struct{}{}
		}
	}
	if len(branches) == 0 {
		return nil
	}
	prs, err := listPRs(ctx, board.SourceRepoPath)
	if err != nil {
		return err
	}
	// `gh pr list` returns most-recent-first, so the first hit per branch wins.
	byBranch := make(map[string]ghPR, len(prs))
	for _, pr := range prs {
		if _, ok := branches[pr.HeadRefName]; !ok {
			continue
		}
		if _, seen := byBranch[pr.HeadRefName]; seen {
			continue
		}
		byBranch[pr.HeadRefName] = pr
	}
	cols, err := p.store.ListColumns(ctx, board.ID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	colByName := make(map[string]*db.Column, len(cols))
	for i := range cols {
		colByName[cols[i].Name] = &cols[i]
	}
	for i := range sessions {
		sess := &sessions[i]
		pr, ok := byBranch[sess.BranchName]
		if !ok {
			continue
		}
		newState := classify(pr)
		if newState == sess.PRState {
			continue
		}
		if err := p.applyTransition(ctx, board, sess, sess.PRState, newState, cfg, colByName); err != nil {
			log.Printf("github poller: ticket %d: %v", sess.TicketID, err)
		}
	}
	return nil
}

func classify(pr ghPR) string {
	switch pr.State {
	case "MERGED":
		return PRStateMerged
	case "CLOSED":
		return PRStateClosed
	case "OPEN":
		if pr.IsDraft {
			return PRStateDraft
		}
		return PRStateOpen
	}
	return ""
}

func columnFor(state string, cfg Config) string {
	switch state {
	case PRStateDraft:
		return cfg.DraftColumn
	case PRStateOpen:
		return cfg.ReviewColumn
	case PRStateMerged:
		return cfg.DoneColumn
	case PRStateClosed:
		return cfg.ClosedColumn
	}
	return ""
}

func (p *Poller) applyTransition(ctx context.Context, board *db.Board, sess *db.Session, prior, next string, cfg Config, colByName map[string]*db.Column) error {
	// Always persist the new observation, even when we decide not to move.
	defer func() {
		if err := p.store.UpdateSessionPRState(ctx, sess.ID, next); err != nil {
			log.Printf("github poller: persist pr_state %d: %v", sess.ID, err)
		}
	}()

	target := columnFor(next, cfg)
	if target == "" {
		return nil
	}
	targetCol, ok := colByName[target]
	if !ok {
		log.Printf("github poller: board %d has no column %q (target for state %q)", board.ID, target, next)
		return nil
	}
	t, err := p.store.GetTicket(ctx, sess.TicketID)
	if err != nil {
		return fmt.Errorf("get ticket: %w", err)
	}
	if t.ArchivedAt != nil {
		return nil
	}
	// Don't fight a manual move: only act when the ticket sits in the column
	// the prior state mapped to. First observation (prior=="") always acts.
	if prior != "" {
		if priorCol := columnFor(prior, cfg); priorCol != "" && priorCol != target {
			if c, ok := colByName[priorCol]; ok && c.ID != t.ColumnID {
				return nil
			}
		}
	}
	if t.ColumnID == targetCol.ID {
		return nil
	}
	maxPos, err := p.store.MaxTicketPosition(ctx, targetCol.ID)
	if err != nil {
		return fmt.Errorf("max position: %w", err)
	}
	if err := p.store.MoveTicket(ctx, t.ID, targetCol.ID, maxPos+1); err != nil {
		return fmt.Errorf("move ticket: %w", err)
	}
	if updated, err := p.store.GetTicket(ctx, t.ID); err == nil {
		p.bus.Publish(board.ID, "ticket_moved", updated)
	}
	if next == PRStateMerged && p.sessions != nil {
		if err := p.sessions.Destroy(ctx, sess.ID); err != nil {
			log.Printf("github poller: destroy session %d after merge: %v", sess.ID, err)
		}
	}
	return nil
}

func listPRs(ctx context.Context, repoPath string) ([]ghPR, error) {
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "gh", "pr", "list",
		"--state", "all",
		"--limit", "200",
		"--json", "headRefName,state,isDraft",
	)
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh pr list: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	if stdout.Len() == 0 {
		return nil, nil
	}
	var prs []ghPR
	if err := json.Unmarshal(stdout.Bytes(), &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	return prs, nil
}
