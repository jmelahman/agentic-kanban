// Package client is a thin HTTP client for the kanban REST API. It's used
// by both the MCP server (internal/mcp) and the kanban CLI subcommands
// (cmd/server) so the request shapes stay in one place.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Client wraps an *http.Client and a base URL.
type Client struct {
	baseURL string
	hc      *http.Client
}

// New returns a Client. If hc is nil, http.DefaultClient is used.
func New(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), hc: hc}
}

// Board mirrors the subset of fields callers need from /api/boards.
type Board struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Ticket mirrors the subset of fields callers need from ticket responses.
type Ticket struct {
	ID       int64  `json:"id"`
	BoardID  int64  `json:"board_id"`
	ColumnID int64  `json:"column_id"`
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Position int    `json:"position"`
}

// CreateBoardArgs is the request body for POST /api/boards. Name plus one of
// repo_path or mount_path are required by the server; everything else is
// optional and falls back to server defaults.
type CreateBoardArgs struct {
	Name           string `json:"name"`
	RepoPath       string `json:"repo_path,omitempty"`
	MountPath      string `json:"mount_path,omitempty"`
	WorktreeRoot   string `json:"worktree_root,omitempty"`
	BaseBranch     string `json:"base_branch,omitempty"`
	BranchPrefix   string `json:"branch_prefix,omitempty"`
	GitAuthorName  string `json:"git_author_name,omitempty"`
	GitAuthorEmail string `json:"git_author_email,omitempty"`
}

// UpdateBoardArgs is the request body for PATCH /api/boards/{id}. Pointer
// fields signal "include in patch"; nil leaves the value untouched.
type UpdateBoardArgs struct {
	Name           *string `json:"name,omitempty"`
	RepoPath       *string `json:"repo_path,omitempty"`
	MountPath      *string `json:"mount_path,omitempty"`
	WorktreeRoot   *string `json:"worktree_root,omitempty"`
	BaseBranch     *string `json:"base_branch,omitempty"`
	BranchPrefix   *string `json:"branch_prefix,omitempty"`
	GitAuthorName  *string `json:"git_author_name,omitempty"`
	GitAuthorEmail *string `json:"git_author_email,omitempty"`
}

// CreateTicketArgs is the input shape for CreateTicket. Only Board and Title
// are required; the server defaults Column to the leftmost column.
type CreateTicketArgs struct {
	Board  string `json:"-"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Column string `json:"column,omitempty"`
}

// UpdateTicketArgs is the request body for PATCH /api/tickets/{id}.
type UpdateTicketArgs struct {
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
}

// MoveTicketArgs is the request body for PATCH /api/tickets/{id}/move.
type MoveTicketArgs struct {
	ColumnID int64 `json:"column_id"`
	Position int   `json:"position"`
}

// ListBoards calls GET /api/boards.
func (c *Client) ListBoards(ctx context.Context) ([]Board, error) {
	raw, err := c.do(ctx, http.MethodGet, "/api/boards", nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var rawBoards []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawBoards); err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(rawBoards))
	for _, b := range rawBoards {
		var s Board
		_ = json.Unmarshal(b["id"], &s.ID)
		_ = json.Unmarshal(b["name"], &s.Name)
		_ = json.Unmarshal(b["slug"], &s.Slug)
		out = append(out, s)
	}
	return out, nil
}

// CreateBoard calls POST /api/boards and returns the raw board JSON.
func (c *Client) CreateBoard(ctx context.Context, a CreateBoardArgs) (json.RawMessage, error) {
	if strings.TrimSpace(a.Name) == "" {
		return nil, fmt.Errorf("name required")
	}
	if a.RepoPath == "" && a.MountPath == "" {
		return nil, fmt.Errorf("repo-path or mount-path required")
	}
	return c.do(ctx, http.MethodPost, "/api/boards", a, http.StatusCreated)
}

// GetBoard calls GET /api/boards/{id}.
func (c *Client) GetBoard(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/api/boards/"+strconv.FormatInt(id, 10), nil, http.StatusOK)
}

// UpdateBoard calls PATCH /api/boards/{id}.
func (c *Client) UpdateBoard(ctx context.Context, id int64, a UpdateBoardArgs) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPatch, "/api/boards/"+strconv.FormatInt(id, 10), a, http.StatusOK)
}

// DeleteBoard calls DELETE /api/boards/{id}.
func (c *Client) DeleteBoard(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/boards/"+strconv.FormatInt(id, 10), nil, http.StatusNoContent)
	return err
}

// BoardState calls GET /api/boards/{id}/state.
func (c *Client) BoardState(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/api/boards/"+strconv.FormatInt(id, 10)+"/state", nil, http.StatusOK)
}

// ListArchived calls GET /api/boards/{id}/archived.
func (c *Client) ListArchived(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/api/boards/"+strconv.FormatInt(id, 10)+"/archived", nil, http.StatusOK)
}

// DeleteArchived calls DELETE /api/boards/{id}/archived.
func (c *Client) DeleteArchived(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/boards/"+strconv.FormatInt(id, 10)+"/archived", nil, http.StatusNoContent)
	return err
}

// CreateTicket calls POST /api/boards/{board}/tickets and returns the raw
// JSON the server responded with. Callers that want a typed value can
// re-decode; we keep the raw form here so both MCP (which forwards to the
// agent) and CLI (which re-prints) can use it.
func (c *Client) CreateTicket(ctx context.Context, a CreateTicketArgs) (json.RawMessage, error) {
	if a.Board == "" {
		return nil, fmt.Errorf("board required")
	}
	if a.Title == "" {
		return nil, fmt.Errorf("title required")
	}
	body := map[string]any{"title": a.Title}
	if a.Body != "" {
		body["body"] = a.Body
	}
	if a.Column != "" {
		body["column"] = a.Column
	}
	return c.do(ctx, http.MethodPost, "/api/boards/"+url.PathEscape(a.Board)+"/tickets", body, http.StatusCreated)
}

// UpdateTicket calls PATCH /api/tickets/{id}.
func (c *Client) UpdateTicket(ctx context.Context, id int64, a UpdateTicketArgs) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPatch, "/api/tickets/"+strconv.FormatInt(id, 10), a, http.StatusOK)
}

// MoveTicket calls PATCH /api/tickets/{id}/move.
func (c *Client) MoveTicket(ctx context.Context, id int64, a MoveTicketArgs) error {
	_, err := c.do(ctx, http.MethodPatch, "/api/tickets/"+strconv.FormatInt(id, 10)+"/move", a, http.StatusNoContent)
	return err
}

// ArchiveTicket calls POST /api/tickets/{id}/archive.
func (c *Client) ArchiveTicket(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodPost, "/api/tickets/"+strconv.FormatInt(id, 10)+"/archive", nil, http.StatusNoContent)
	return err
}

// UnarchiveTicket calls POST /api/tickets/{id}/unarchive.
func (c *Client) UnarchiveTicket(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodPost, "/api/tickets/"+strconv.FormatInt(id, 10)+"/unarchive", nil, http.StatusNoContent)
	return err
}

// DeleteTicket calls DELETE /api/tickets/{id}. The server requires the
// ticket to be archived first; callers should propagate the resulting error.
func (c *Client) DeleteTicket(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/tickets/"+strconv.FormatInt(id, 10), nil, http.StatusNoContent)
	return err
}

// DoneTicket calls POST /api/tickets/{id}/done.
func (c *Client) DoneTicket(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodPost, "/api/tickets/"+strconv.FormatInt(id, 10)+"/done", nil, http.StatusNoContent)
	return err
}

// SyncTicket calls POST /api/tickets/{id}/sync. Strategy is "rebase" or "merge".
func (c *Client) SyncTicket(ctx context.Context, id int64, strategy string) error {
	_, err := c.do(ctx, http.MethodPost, "/api/tickets/"+strconv.FormatInt(id, 10)+"/sync",
		map[string]string{"strategy": strategy}, http.StatusNoContent)
	return err
}

// MergeTicket calls POST /api/tickets/{id}/merge. Strategy is one of
// "merge-commit", "squash", or "rebase".
func (c *Client) MergeTicket(ctx context.Context, id int64, strategy string) error {
	_, err := c.do(ctx, http.MethodPost, "/api/tickets/"+strconv.FormatInt(id, 10)+"/merge",
		map[string]string{"strategy": strategy}, http.StatusNoContent)
	return err
}

// ArchiveColumnTickets calls POST /api/columns/{id}/archive-all.
func (c *Client) ArchiveColumnTickets(ctx context.Context, columnID int64) error {
	_, err := c.do(ctx, http.MethodPost, "/api/columns/"+strconv.FormatInt(columnID, 10)+"/archive-all", nil, http.StatusNoContent)
	return err
}

// EnsureSession calls POST /api/tickets/{id}/session and returns the raw
// session JSON.
func (c *Client) EnsureSession(ctx context.Context, ticketID int64) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPost, "/api/tickets/"+strconv.FormatInt(ticketID, 10)+"/session", nil, http.StatusCreated)
}

// StartSession calls POST /api/sessions/{id}/start.
func (c *Client) StartSession(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPost, "/api/sessions/"+strconv.FormatInt(id, 10)+"/start", nil, http.StatusOK)
}

// StopSession calls POST /api/sessions/{id}/stop.
func (c *Client) StopSession(ctx context.Context, id int64) error {
	_, err := c.do(ctx, http.MethodPost, "/api/sessions/"+strconv.FormatInt(id, 10)+"/stop", nil, http.StatusNoContent)
	return err
}

// RestartSession calls POST /api/sessions/{id}/restart.
func (c *Client) RestartSession(ctx context.Context, id int64) (json.RawMessage, error) {
	return c.do(ctx, http.MethodPost, "/api/sessions/"+strconv.FormatInt(id, 10)+"/restart", nil, http.StatusOK)
}

// ConfigPatchArgs is the request body for PATCH /api/config. Scope is "local"
// or "global"; Board (id or slug) is required for local scope. Set values are
// native typed values (bool/string/array/object); Unset names keys to clear.
type ConfigPatchArgs struct {
	Scope string         `json:"scope"`
	Board string         `json:"board,omitempty"`
	Set   map[string]any `json:"set,omitempty"`
	Unset []string       `json:"unset,omitempty"`
}

// Config calls GET /api/config. scope is "effective" (default when ""),
// "local", or "global"; board (id or slug) scopes the local layer.
func (c *Client) Config(ctx context.Context, scope, board string) (json.RawMessage, error) {
	q := url.Values{}
	if scope != "" {
		q.Set("scope", scope)
	}
	if board != "" {
		q.Set("board", board)
	}
	path := "/api/config"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	return c.do(ctx, http.MethodGet, path, nil, http.StatusOK)
}

// ConfigPatch calls PATCH /api/config and returns the refreshed config view.
func (c *Client) ConfigPatch(ctx context.Context, a ConfigPatchArgs) (json.RawMessage, error) {
	if a.Scope == "" {
		return nil, fmt.Errorf("scope required (local or global)")
	}
	if len(a.Set) == 0 && len(a.Unset) == 0 {
		return nil, fmt.Errorf("nothing to set or unset")
	}
	return c.do(ctx, http.MethodPatch, "/api/config", a, http.StatusOK)
}

// ResolveBoardID resolves a board identifier (numeric id or slug) to a
// numeric id by listing boards. The REST endpoints under /api/boards/{id}
// (state, archived, etc.) require numeric ids; this helper lets callers
// accept slugs without each doing their own lookup.
func (c *Client) ResolveBoardID(ctx context.Context, ident string) (int64, error) {
	if ident == "" {
		return 0, fmt.Errorf("board identifier required")
	}
	if id, err := strconv.ParseInt(ident, 10, 64); err == nil {
		return id, nil
	}
	boards, err := c.ListBoards(ctx)
	if err != nil {
		return 0, err
	}
	for _, b := range boards {
		if b.Slug == ident || b.Name == ident {
			return b.ID, nil
		}
	}
	return 0, fmt.Errorf("board %q not found", ident)
}

// do is the shared request helper. body is JSON-encoded if non-nil; the
// response body is returned only when expectStatus matches and the server
// returned a body (HEAD/204 paths return nil, nil).
func (c *Client) do(ctx context.Context, method, path string, body any, expectStatus int) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectStatus {
		return nil, c.readError(resp)
	}
	if resp.StatusCode == http.StatusNoContent || resp.ContentLength == 0 {
		return nil, nil
	}
	return io.ReadAll(resp.Body)
}

func (c *Client) readError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != "" {
		return fmt.Errorf("%s: %s", resp.Status, e.Error)
	}
	return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
}
