package session

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"

	"github.com/gorilla/websocket"

	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
)

// replayBufferSize bounds the recent-output buffer that gets replayed to
// reconnecting clients. 64 KiB comfortably covers a screenful or two of
// output for a typical terminal.
const replayBufferSize = 64 * 1024

// ringBuffer retains the most-recent bytes written to it, up to a fixed
// capacity. Not safe for concurrent use; the caller serializes access.
type ringBuffer struct {
	buf  []byte
	n    int // number of valid bytes (clamped at len(buf))
	head int // next write index
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{buf: make([]byte, size)}
}

func (r *ringBuffer) Write(p []byte) {
	if len(r.buf) == 0 {
		return
	}
	if len(p) >= len(r.buf) {
		copy(r.buf, p[len(p)-len(r.buf):])
		r.head = 0
		r.n = len(r.buf)
		return
	}
	for len(p) > 0 {
		space := len(r.buf) - r.head
		n := len(p)
		if n > space {
			n = space
		}
		copy(r.buf[r.head:], p[:n])
		r.head = (r.head + n) % len(r.buf)
		if r.n < len(r.buf) {
			r.n += n
			if r.n > len(r.buf) {
				r.n = len(r.buf)
			}
		}
		p = p[n:]
	}
}

// Snapshot returns the buffered bytes in oldest-first order.
func (r *ringBuffer) Snapshot() []byte {
	if r.n < len(r.buf) {
		out := make([]byte, r.n)
		copy(out, r.buf[:r.n])
		return out
	}
	out := make([]byte, len(r.buf))
	n := copy(out, r.buf[r.head:])
	copy(out[n:], r.buf[:r.head])
	return out
}

// sessionPTY brokers a single docker exec PTY for a session. It owns the
// hijacked exec connection and survives WebSocket attach/detach so that
// clients can reconnect (e.g. after a page refresh) without killing the
// underlying claude process.
//
// The single reader goroutine is the only writer of binary frames to the
// active client. WS handlers call register/unregister/write/resize.
type sessionPTY struct {
	sessionID int64
	attached  *docker.AttachedExec
	docker    *docker.Client
	set       *brokerSet

	mu     sync.Mutex
	buf    *ringBuffer
	cols   uint
	rows   uint
	client *websocket.Conn
	closed bool
}

// brokerSet manages the set of active brokers, one per session id.
type brokerSet struct {
	docker *docker.Client

	mu      sync.Mutex
	perSess map[int64]*sessionPTY
}

func newBrokerSet(dc *docker.Client) *brokerSet {
	return &brokerSet{
		docker:  dc,
		perSess: map[int64]*sessionPTY{},
	}
}

// attach returns the broker for this session, creating it (and starting the
// underlying docker exec) on first use. Subsequent calls return the existing
// broker — the cmd and workDir arguments are only honored at creation time.
func (s *brokerSet) attach(ctx context.Context, sess *db.Session, cmd []string, workDir string) (*sessionPTY, error) {
	s.mu.Lock()
	if b, ok := s.perSess[sess.ID]; ok {
		s.mu.Unlock()
		return b, nil
	}
	if sess.ContainerID == nil || *sess.ContainerID == "" {
		s.mu.Unlock()
		return nil, errors.New("session not running")
	}
	att, err := s.docker.ExecAttachTTY(ctx, *sess.ContainerID, cmd, workDir, nil)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	b := &sessionPTY{
		sessionID: sess.ID,
		attached:  att,
		docker:    s.docker,
		set:       s,
		buf:       newRingBuffer(replayBufferSize),
	}
	s.perSess[sess.ID] = b
	s.mu.Unlock()
	go b.readLoop()
	return b, nil
}

// closeFor tears down the broker for a session, if any. Idempotent and safe
// when no broker exists.
func (s *brokerSet) closeFor(sessionID int64) {
	s.mu.Lock()
	b := s.perSess[sessionID]
	s.mu.Unlock()
	if b != nil {
		b.shutdown()
	}
}

// readLoop pumps bytes from the docker exec into the ring buffer and the
// active client. On read error / EOF it triggers a full shutdown.
func (b *sessionPTY) readLoop() {
	defer b.shutdown()
	buf := make([]byte, 4096)
	for {
		n, err := b.attached.Conn.Reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			b.mu.Lock()
			if b.closed {
				b.mu.Unlock()
				return
			}
			b.buf.Write(chunk)
			ws := b.client
			if ws != nil {
				if werr := ws.WriteMessage(websocket.BinaryMessage, chunk); werr != nil {
					if b.client == ws {
						b.client = nil
					}
					_ = ws.Close()
				}
			}
			b.mu.Unlock()
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("session %d pty read: %v", b.sessionID, err)
			}
			return
		}
	}
}

// shutdown notifies the active client (if any), closes the hijacked exec
// connection, and removes the broker from the set. Idempotent.
func (b *sessionPTY) shutdown() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	ws := b.client
	b.client = nil
	b.mu.Unlock()

	if ws != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n[session ended]\r\n"))
		_ = ws.Close()
	}
	_ = b.attached.Conn.Conn.Close()

	b.set.mu.Lock()
	if cur, ok := b.set.perSess[b.sessionID]; ok && cur == b {
		delete(b.set.perSess, b.sessionID)
	}
	b.set.mu.Unlock()
}

// register makes ws the active client. Any prior client is kicked. The
// recent output buffer is replayed to ws before this returns.
func (b *sessionPTY) register(ws *websocket.Conn) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return errors.New("session ended")
	}
	old := b.client
	if old != nil && old != ws {
		_ = old.WriteMessage(websocket.TextMessage, []byte("\r\n[replaced by another window]\r\n"))
		_ = old.Close()
	}
	b.client = ws
	snap := b.buf.Snapshot()
	if len(snap) > 0 {
		if err := ws.WriteMessage(websocket.BinaryMessage, snap); err != nil {
			b.client = nil
			return err
		}
	}
	return nil
}

// unregister clears ws as the active client, but only if it still matches —
// avoids racing with a concurrent register that already swapped in a newer
// client.
func (b *sessionPTY) unregister(ws *websocket.Conn) {
	b.mu.Lock()
	if b.client == ws {
		b.client = nil
	}
	b.mu.Unlock()
}

// write forwards stdin from the active client to the docker exec. Writes
// from a non-current client (kicked) are silently dropped.
func (b *sessionPTY) write(from *websocket.Conn, p []byte) error {
	b.mu.Lock()
	if b.closed || b.client != from {
		b.mu.Unlock()
		return nil
	}
	conn := b.attached.Conn.Conn
	b.mu.Unlock()
	_, err := conn.Write(p)
	return err
}

// resize forwards a TTY resize and remembers the size for any subsequent
// reattach. Resizes from a non-current client are dropped.
func (b *sessionPTY) resize(ctx context.Context, from *websocket.Conn, cols, rows uint) error {
	b.mu.Lock()
	if b.closed || b.client != from {
		b.mu.Unlock()
		return nil
	}
	b.cols = cols
	b.rows = rows
	execID := b.attached.ID
	b.mu.Unlock()
	return b.docker.ResizeExec(ctx, execID, cols, rows)
}
