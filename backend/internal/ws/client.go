package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const (
	sendBuffer   = 256
	writeTimeout = 10 * time.Second
	pingInterval = 30 * time.Second
)

type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	userID  uuid.UUID
	send    chan []byte
	session uuid.UUID // guarded by hub.mu
	log     *slog.Logger
	closed  chan struct{}
}

func (c *Client) sendRaw(b []byte) {
	select {
	case c.send <- b:
	default:
		// slow consumer: drop the connection; the client will reconnect and refetch.
		c.log.Warn("ws client too slow, closing", "user", c.userID)
		go c.conn.Close(websocket.StatusPolicyViolation, "too slow")
	}
}

func (c *Client) sendEvent(typ string, data any) {
	b, err := encode(typ, data)
	if err == nil {
		c.sendRaw(b)
	}
}

// Serve upgrades the request and runs the connection until it closes.
// userID must already be authenticated by the caller.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, userID uuid.UUID, originPatterns []string) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: originPatterns})
	if err != nil {
		h.log.Warn("ws accept", "err", err)
		return
	}
	conn.SetReadLimit(64 * 1024)
	c := &Client{hub: h, conn: conn, userID: userID, send: make(chan []byte, sendBuffer), log: h.log, closed: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.writeLoop(ctx)
	c.readLoop(ctx)
	h.remove(c)
	close(c.closed)
	conn.Close(websocket.StatusNormalClosure, "")
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) == -1 {
				c.log.Debug("ws read", "err", err)
			}
			return
		}
		var env Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			c.sendEvent(TypeError, map[string]string{"code": "bad_request", "message": "invalid envelope"})
			continue
		}
		switch env.Type {
		case "ping":
			c.sendEvent(TypePong, struct{}{})
		case "subscribe":
			var d struct {
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(env.Data, &d)
			sid, err := uuid.Parse(d.SessionID)
			if err != nil || !c.hub.authorize(ctx, c.userID, sid) {
				c.sendEvent(TypeError, map[string]string{"code": "forbidden", "message": "cannot subscribe to session"})
				continue
			}
			c.hub.subscribe(c, sid)
		case "typing":
			// User typing indicators are accepted but not used in AI-Solo mode.
		default:
			c.sendEvent(TypeError, map[string]string{"code": "unknown_type", "message": "unknown message type: " + env.Type})
		}
	}
}

func (c *Client) writeLoop(ctx context.Context) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		case b := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				c.conn.Close(websocket.StatusGoingAway, "write failed")
				return
			}
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil {
				c.conn.Close(websocket.StatusGoingAway, "ping failed")
				return
			}
		}
	}
}
