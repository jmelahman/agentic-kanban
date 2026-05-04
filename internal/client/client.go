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

// CreateTicketArgs is the input shape for CreateTicket. Only Board and Title
// are required; the server defaults Column to the leftmost column.
type CreateTicketArgs struct {
	Board  string `json:"-"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Column string `json:"column,omitempty"`
}

// ListBoards calls GET /api/boards.
func (c *Client) ListBoards(ctx context.Context) ([]Board, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/boards", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, c.readError(resp)
	}
	var raw []map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(raw))
	for _, b := range raw {
		var s Board
		_ = json.Unmarshal(b["id"], &s.ID)
		_ = json.Unmarshal(b["name"], &s.Name)
		_ = json.Unmarshal(b["slug"], &s.Slug)
		out = append(out, s)
	}
	return out, nil
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
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	endpoint := c.baseURL + "/api/boards/" + url.PathEscape(a.Board) + "/tickets"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return nil, c.readError(resp)
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
