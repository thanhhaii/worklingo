// Package ws implements the WebSocket envelope protocol and per-session broadcast.
package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

// Envelope is the wire format in both directions.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Server -> client event types.
const (
	TypePong              = "pong"
	TypeMessageNew        = "message.new"
	TypeMessageDelta      = "message.delta"
	TypeMessageDone       = "message.done"
	TypeTyping            = "typing"
	TypeTaskNew           = "task.new"
	TypeTaskUpdated       = "task.updated"
	TypeDeadlineNew       = "deadline.new"
	TypeDeadlineUpdated   = "deadline.updated"
	TypeWarningNew        = "warning.new"
	TypeSessionUpdated    = "session.updated"
	TypeEvaluationUpdated = "evaluation.updated"
	TypeError             = "error"
)

// Authorizer reports whether user may subscribe to session.
type Authorizer func(ctx context.Context, userID, sessionID uuid.UUID) bool

// Broadcaster is the interface used by domain packages.
type Broadcaster interface {
	Broadcast(sessionID uuid.UUID, typ string, data any)
}

type Hub struct {
	log       *slog.Logger
	authorize Authorizer

	mu   sync.RWMutex
	subs map[uuid.UUID]map[*Client]struct{}
}

func NewHub(log *slog.Logger, authorize Authorizer) *Hub {
	return &Hub{log: log, authorize: authorize, subs: map[uuid.UUID]map[*Client]struct{}{}}
}

func encode(typ string, data any) ([]byte, error) {
	if data == nil {
		data = struct{}{}
	}
	d, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Type: typ, Data: d})
}

// Broadcast sends an event to every client subscribed to sessionID.
func (h *Hub) Broadcast(sessionID uuid.UUID, typ string, data any) {
	b, err := encode(typ, data)
	if err != nil {
		h.log.Error("ws encode", "type", typ, "err", err)
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.subs[sessionID] {
		c.sendRaw(b)
	}
}

func (h *Hub) subscribe(c *Client, sessionID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.session != uuid.Nil {
		if set := h.subs[c.session]; set != nil {
			delete(set, c)
			if len(set) == 0 {
				delete(h.subs, c.session)
			}
		}
	}
	c.session = sessionID
	set := h.subs[sessionID]
	if set == nil {
		set = map[*Client]struct{}{}
		h.subs[sessionID] = set
	}
	set[c] = struct{}{}
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.subs[c.session]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.subs, c.session)
		}
	}
}
