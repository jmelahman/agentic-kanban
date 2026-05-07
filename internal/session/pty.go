package session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/jmelahman/kanban/internal/db"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type ptyControl struct {
	Type string `json:"type"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Data string `json:"data,omitempty"`
}

// AttachAgent upgrades the request to a WebSocket and routes it through the
// per-session agent PTY broker, which holds the docker exec connection across
// client reconnects (e.g. page refresh). The command argument is the agent
// CLI argv chosen by the caller (typically derived from the global harness
// setting); it must be non-empty.
func (m *Manager) AttachAgent(ctx context.Context, sess *db.Session, w http.ResponseWriter, r *http.Request, command []string, workDir string) error {
	return m.attachKind(ctx, sess, w, r, "agent", command, workDir)
}

// AttachShell upgrades the request to a WebSocket and runs an interactive
// shell inside the session container, brokered alongside (and independent of)
// the agent PTY. The container's user-configured login shell (as recorded in
// /etc/passwd, falling back to /bin/sh) is used so the choice tracks whatever
// the image declares — no need to hardcode bash here.
func (m *Manager) AttachShell(ctx context.Context, sess *db.Session, w http.ResponseWriter, r *http.Request, workDir string) error {
	return m.attachKind(ctx, sess, w, r, "shell", []string{"sh", "-c", `s=$(getent passwd "$(id -u)" 2>/dev/null | cut -d: -f7); exec "${s:-/bin/sh}"`}, workDir)
}

func (m *Manager) attachKind(ctx context.Context, sess *db.Session, w http.ResponseWriter, r *http.Request, kind string, command []string, workDir string) error {
	if sess.ContainerID == nil || *sess.ContainerID == "" {
		http.Error(w, "session not running", http.StatusBadRequest)
		return errors.New("not running")
	}
	if len(command) == 0 {
		http.Error(w, "no command configured", http.StatusBadRequest)
		return errors.New("no command configured")
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	broker, err := m.brokers.attach(ctx, sess, kind, command, workDir)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
		return err
	}
	if err := broker.register(conn); err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("error: "+err.Error()))
		return err
	}
	defer broker.unregister(conn)

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return nil
		}
		switch msgType {
		case websocket.TextMessage:
			var ctl ptyControl
			if err := json.Unmarshal(data, &ctl); err == nil && ctl.Type == "resize" {
				_ = broker.resize(ctx, conn, uint(ctl.Cols), uint(ctl.Rows))
				continue
			}
			if err := broker.write(conn, data); err != nil {
				return nil
			}
		case websocket.BinaryMessage:
			if err := broker.write(conn, data); err != nil {
				return nil
			}
		}
	}
}
