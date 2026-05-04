// Package mcp implements a minimal Model Context Protocol server (stdio
// transport) that lets AI agents create kanban tickets via the existing HTTP
// API. It speaks the JSON-RPC 2.0 dialect described in the MCP spec
// (https://modelcontextprotocol.io) — initialize, tools/list, tools/call,
// plus the standard cancellation / ping flow. Only the methods we need are
// implemented; unknown methods return JSON-RPC error -32601.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/jmelahman/kanban/internal/client"
)

// Protocol version we advertise. Clients can still negotiate; we just echo
// back whatever they asked for if it's something we recognize, otherwise we
// pin to this. Kept current with the published MCP spec.
const protocolVersion = "2025-06-18"

// Run starts the MCP server on stdin/stdout and blocks until ctx is done or
// stdin closes. serverURL is the base URL of a running `kanban serve`.
func Run(ctx context.Context, serverURL string) error {
	return run(ctx, serverURL, os.Stdin, os.Stdout, http.DefaultClient)
}

func run(ctx context.Context, serverURL string, in io.Reader, out io.Writer, hc *http.Client) error {
	srv := &server{client: client.New(serverURL, hc), out: out}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		srv.handle(ctx, line)
	}
	return scanner.Err()
}

type server struct {
	client *client.Client
	out    io.Writer
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (s *server) handle(ctx context.Context, line []byte) {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		s.write(rpcResponse{JSONRPC: "2.0", ID: nil, Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
		return
	}
	// Notifications (no id) must not get a response.
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	switch req.Method {
	case "initialize":
		if isNotification {
			return
		}
		s.write(s.success(req.ID, s.initialize(req.Params)))
	case "notifications/initialized", "notifications/cancelled":
		// fire-and-forget
	case "ping":
		if isNotification {
			return
		}
		s.write(s.success(req.ID, map[string]any{}))
	case "tools/list":
		if isNotification {
			return
		}
		s.write(s.success(req.ID, map[string]any{"tools": toolDefinitions()}))
	case "tools/call":
		if isNotification {
			return
		}
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			s.write(rpcResponse{JSONRPC: "2.0", ID: rawID(req.ID), Error: &rpcError{Code: -32603, Message: err.Error()}})
			return
		}
		s.write(s.success(req.ID, result))
	default:
		if isNotification {
			return
		}
		s.write(rpcResponse{JSONRPC: "2.0", ID: rawID(req.ID), Error: &rpcError{Code: -32601, Message: "method not found: " + req.Method}})
	}
}

func (s *server) success(id json.RawMessage, result any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: rawID(id), Result: result}
}

// rawID returns the id as a typed value so json.Marshal preserves number vs
// string vs null. Without this we'd quote numeric ids.
func rawID(id json.RawMessage) any {
	if len(id) == 0 {
		return nil
	}
	return id
}

func (s *server) write(resp rpcResponse) {
	buf, err := json.Marshal(resp)
	if err != nil {
		return
	}
	buf = append(buf, '\n')
	_, _ = s.out.Write(buf)
}

func (s *server) initialize(params json.RawMessage) map[string]any {
	var req struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &req)
	version := req.ProtocolVersion
	if version == "" {
		version = protocolVersion
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "kanban",
			"version": "0.1.0",
		},
	}
}

func toolDefinitions() []map[string]any {
	return []map[string]any{
		{
			"name":        "create_ticket",
			"description": "Create a new ticket on a kanban board. The board can be referenced by numeric id or slug. Column is optional and defaults to the leftmost column (typically Backlog); when set it accepts a column name (case-insensitive) or numeric id.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"board":  map[string]any{"type": "string", "description": "Board id or slug"},
					"title":  map[string]any{"type": "string", "description": "Ticket title"},
					"body":   map[string]any{"type": "string", "description": "Optional ticket body / description"},
					"column": map[string]any{"type": "string", "description": "Optional column name or id"},
				},
				"required": []string{"board", "title"},
			},
		},
		{
			"name":        "list_boards",
			"description": "List all kanban boards as {id, name, slug}. Useful for discovering which board to target before calling create_ticket.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

func (s *server) callTool(ctx context.Context, params json.RawMessage) (map[string]any, error) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	switch p.Name {
	case "create_ticket":
		var args struct {
			Board  string `json:"board"`
			Title  string `json:"title"`
			Body   string `json:"body"`
			Column string `json:"column"`
		}
		if len(p.Arguments) > 0 {
			if err := json.Unmarshal(p.Arguments, &args); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		raw, err := s.client.CreateTicket(ctx, client.CreateTicketArgs{
			Board:  args.Board,
			Title:  args.Title,
			Body:   args.Body,
			Column: args.Column,
		})
		if err != nil {
			return errorResult(err.Error()), nil
		}
		return textResult(string(raw)), nil
	case "list_boards":
		boards, err := s.client.ListBoards(ctx)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		buf, _ := json.Marshal(boards)
		return textResult(string(buf)), nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", p.Name)
	}
}

func textResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

func errorResult(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": true,
	}
}
