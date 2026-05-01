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

// AttachClaude upgrades the request to a WebSocket and routes it through the
// per-session PTY broker, which holds the docker exec connection across
// client reconnects (e.g. page refresh).
func (m *Manager) AttachClaude(ctx context.Context, sess *db.Session, w http.ResponseWriter, r *http.Request, command []string, workDir string) error {
	if sess.ContainerID == nil || *sess.ContainerID == "" {
		http.Error(w, "session not running", http.StatusBadRequest)
		return errors.New("not running")
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	if len(command) == 0 {
		command = []string{"claude"}
	}

	broker, err := m.brokers.attach(ctx, sess, command, workDir)
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
