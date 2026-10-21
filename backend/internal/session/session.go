// Package session manages the AI-Solo session lifecycle.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/evaluation"
	"github.com/worklingo/backend/internal/lifecycle"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/scenario"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/workctx"
	"github.com/worklingo/backend/internal/ws"
)

var ErrActiveExists = errors.New("user already has an active session")

// Channel names created for every session.
var ChannelNames = []string{"general", "sprint-planning", "incident-room", "watercooler"}

type Manager struct {
	pool      *pgxpool.Pool
	q         *store.Queries
	reg       *lifecycle.Registry
	engine    *persona.Engine
	scheduler *scenario.Scheduler
	deadlines *deadline.Service
	eval      *evaluation.Service
	wctx      *workctx.Generator
	hub       ws.Broadcaster
	duration  time.Duration
	log       *slog.Logger

	mu     sync.Mutex
	timers map[uuid.UUID]*time.Timer
}

func NewManager(pool *pgxpool.Pool, q *store.Queries, reg *lifecycle.Registry, e *persona.Engine, sch *scenario.Scheduler,
	d *deadline.Service, ev *evaluation.Service, wc *workctx.Generator, hub ws.Broadcaster, duration time.Duration, log *slog.Logger) *Manager {
	return &Manager{pool: pool, q: q, reg: reg, engine: e, scheduler: sch, deadlines: d, eval: ev, wctx: wc, hub: hub,
		duration: duration, log: log, timers: map[uuid.UUID]*time.Timer{}}
}

// Create starts a new session with channels, DMs and a greeting.
func (m *Manager) Create(ctx context.Context, user store.User) (store.Session, error) {
	now := time.Now()
	first := m.scheduler.FirstAt(now)
	// Generate the work context before the transaction (LLM call; falls back to static).
	wcRaw, _ := json.Marshal(m.wctx.Generate(ctx))
	var sess store.Session
	var general store.Channel
	err := pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		q := m.q.WithTx(tx)
		var err error
		sess, err = q.CreateSession(ctx, store.CreateSessionParams{UserID: user.ID, ExpiresAt: now.Add(m.duration), NextTaskAt: &first, WorkContext: wcRaw})
		if err != nil {
			var pe *pgconn.PgError
			if errors.As(err, &pe) && pe.Code == "23505" {
				return ErrActiveExists
			}
			return err
		}
		pos := int32(0)
		for _, name := range ChannelNames {
			c, err := q.CreateChannel(ctx, store.CreateChannelParams{SessionID: sess.ID, Kind: "CHANNEL", Name: name, Position: pos})
			if err != nil {
				return err
			}
			if name == "general" {
				general = c
			}
			pos++
		}
		for _, pid := range persona.Order {
			p := pid
			if _, err := q.CreateChannel(ctx, store.CreateChannelParams{SessionID: sess.ID, Kind: "DM", Name: pid, PersonaID: &p, Position: pos}); err != nil {
				return err
			}
			pos++
		}
		return nil
	})
	if err != nil {
		return sess, err
	}
	sctx := m.activate(sess)

	// Greeting from Sarah in #general.
	go func() {
		time.Sleep(1500 * time.Millisecond) // give the client a moment to subscribe
		if sctx.Err() != nil {
			return
		}
		p, _ := m.engine.Catalog().Get("sarah")
		text := persona.Render(p.Greeting, user.Username)
		if strings.TrimSpace(text) == "" {
			return
		}
		if _, err := m.engine.PostStatic(sctx, general, user.Username, "sarah", text); err != nil {
			m.log.Error("post greeting", "err", err)
		}
	}()
	m.log.Info("session started", "session", sess.ID, "user", user.ID, "expiresAt", sess.ExpiresAt)
	return sess, nil
}

// activate registers the session context, expiry timer and task scheduler.
func (m *Manager) activate(sess store.Session) context.Context {
	sctx := m.reg.Start(sess.ID)
	id := sess.ID
	m.mu.Lock()
	if old, ok := m.timers[id]; ok {
		old.Stop()
	}
	m.timers[id] = time.AfterFunc(time.Until(sess.ExpiresAt), func() {
		if _, err := m.End(context.Background(), id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			m.log.Error("expire session", "session", id, "err", err)
		}
	})
	m.mu.Unlock()
	m.scheduler.Run(sctx, sess.ID, sess.NextTaskAt)
	return sctx
}

// End completes an active session (idempotent: returns pgx.ErrNoRows if not active).
func (m *Manager) End(ctx context.Context, id uuid.UUID) (store.Session, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	sess, err := m.q.CompleteSession(ctx, id)
	if err != nil {
		return sess, err
	}
	m.stop(id)
	m.hub.Broadcast(id, ws.TypeSessionUpdated, dto.FromSession(sess))
	m.eval.Start(id)
	m.log.Info("session completed", "session", id)
	return sess, nil
}

func (m *Manager) stop(id uuid.UUID) {
	m.mu.Lock()
	if t, ok := m.timers[id]; ok {
		t.Stop()
		delete(m.timers, id)
	}
	m.mu.Unlock()
	m.deadlines.StopSession(id)
	m.reg.Stop(id)
}

// Recover re-activates ACTIVE sessions after a restart; expired ones are ended.
func (m *Manager) Recover(ctx context.Context) error {
	ss, err := m.q.ListActiveSessions(ctx)
	if err != nil {
		return err
	}
	for _, s := range ss {
		if time.Now().After(s.ExpiresAt) {
			if _, err := m.End(ctx, s.ID); err != nil {
				m.log.Error("end expired session", "session", s.ID, "err", err)
			}
			continue
		}
		m.activate(s)
	}
	m.log.Info("recovered active sessions", "count", len(ss))
	return nil
}

// Shutdown stops all timers (sessions remain ACTIVE in DB for recovery).
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.timers {
		t.Stop()
		delete(m.timers, id)
	}
}
