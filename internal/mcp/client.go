package mcp

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

// kanbanClient is a tiny HTTP client for the kanban server. Tool handlers are
// thin wrappers over the existing REST surface; see internal/api for the
// endpoint definitions.
type kanbanClient struct {
	baseURL string
	hc      *http.Client
}

func newKanbanClient(baseURL string, hc *http.Client) *kanbanClient {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &kanbanClient{baseURL: strings.TrimRight(baseURL, "/"), hc: hc}
}

type boardSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (c *kanbanClient) listBoards(ctx context.Context) ([]boardSummary, error) {
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
	out := make([]boardSummary, 0, len(raw))
	for _, b := range raw {
		var s boardSummary
		_ = json.Unmarshal(b["id"], &s.ID)
		_ = json.Unmarshal(b["name"], &s.Name)
		_ = json.Unmarshal(b["slug"], &s.Slug)
		out = append(out, s)
	}
	return out, nil
}

type createTicketArgs struct {
	Board  string `json:"board"`
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Column string `json:"column,omitempty"`
}

func (c *kanbanClient) createTicket(ctx context.Context, a createTicketArgs) (json.RawMessage, error) {
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

func (c *kanbanClient) readError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(b, &e) == nil && e.Error != "" {
		return fmt.Errorf("%s: %s", resp.Status, e.Error)
	}
	return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
}
