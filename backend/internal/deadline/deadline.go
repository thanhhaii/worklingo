// Package deadline implements the server-authoritative response deadlines.
// The database is the source of truth; in-memory timers fire expirations and
// are rebuilt from PENDING rows on startup.
package deadline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/lifecycle"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/validator"
	"github.com/worklingo/backend/internal/ws"
)

type Service struct {
	q       *store.Queries
	chat    *chat.Service
	hub     ws.Broadcaster
	val     *validator.Validator
	reg     *lifecycle.Registry
	timeout time.Duration
	log     *slog.Logger

	mu     sync.Mutex
	timers map[uuid.UUID]timerEntry
}

type timerEntry struct {
	t         *time.Timer
	sessionID uuid.UUID
}

func NewService(q *store.Queries, c *chat.Service, hub ws.Broadcaster, val *validator.Validator, reg *lifecycle.Registry, timeout time.Duration, log *slog.Logger) *Service {
	return &Service{q: q, chat: c, hub: hub, val: val, reg: reg, timeout: timeout, log: log, timers: map[uuid.UUID]timerEntry{}}
}

// Timeout is the configured response window.
func (s *Service) Timeout() time.Duration { return s.timeout }

// Create inserts a PENDING deadline for messageID and starts its timer.
func (s *Service) Create(ctx context.Context, sessionID, channelID, messageID uuid.UUID, taskID *uuid.UUID) (store.Deadline, error) {
	d, err := s.q.CreateDeadline(ctx, store.CreateDeadlineParams{
		SessionID: sessionID, ChannelID: channelID, MessageID: messageID, TaskID: taskID,
		DeadlineAt: time.Now().Add(s.timeout),
	})
	if err != nil {
		return d, err
	}
	s.hub.Broadcast(sessionID, ws.TypeDeadlineNew, dto.FromDeadline(d))
	s.schedule(d)
	return d, nil
}

// HasPending reports whether the channel already has a pending deadline.
func (s *Service) HasPending(ctx context.Context, channelID uuid.UUID) bool {
	ds, err := s.q.ListPendingDeadlinesForChannel(ctx, channelID)
	return err == nil && len(ds) > 0
}

func (s *Service) schedule(d store.Deadline) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, ok := s.timers[d.ID]; ok {
		old.t.Stop()
	}
	id := d.ID
	t := time.AfterFunc(time.Until(d.DeadlineAt), func() { s.expire(id) })
	s.timers[d.ID] = timerEntry{t: t, sessionID: d.SessionID}
}

func (s *Service) unschedule(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.timers[id]; ok {
		e.t.Stop()
		delete(s.timers, id)
	}
}

// StopSession stops all in-memory timers of a session (rows stay PENDING).
func (s *Service) StopSession(sessionID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range s.timers {
		if e.sessionID == sessionID {
			e.t.Stop()
			delete(s.timers, id)
		}
	}
}

// StopAll stops every timer (shutdown).
func (s *Service) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, e := range s.timers {
		e.t.Stop()
		delete(s.timers, id)
	}
}

// Recover reschedules all PENDING deadlines of ACTIVE sessions.
// Deadlines already past are expired immediately.
func (s *Service) Recover(ctx context.Context) error {
	ds, err := s.q.ListPendingDeadlines(ctx)
	if err != nil {
		return err
	}
	for _, d := range ds {
		s.schedule(d)
	}
	s.log.Info("recovered pending deadlines", "count", len(ds))
	return nil
}

func (s *Service) expire(id uuid.UUID) {
	s.unschedule(id)
	ctx, cancel := context.WithTimeout(s.reg.Root(), 15*time.Second)
	defer cancel()

	d, err := s.q.GetDeadline(ctx, id)
	if err != nil {
		s.log.Error("expire: load deadline", "id", id, "err", err)
		return
	}
	sess, err := s.q.GetSession(ctx, d.SessionID)
	if err != nil || sess.Status != "ACTIVE" {
		return
	}
	d, err = s.q.ExpireDeadline(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return // already resolved
	}
	if err != nil {
		s.log.Error("expire deadline", "id", id, "err", err)
		return
	}
	reason := fmt.Sprintf("No valid reply within %s.", humanDuration(s.timeout))
	if d.TaskID != nil {
		reason = fmt.Sprintf("Task request left unanswered for %s.", humanDuration(s.timeout))
	}
	w, err := s.q.CreateWarning(ctx, store.CreateWarningParams{SessionID: d.SessionID, DeadlineID: d.ID, Reason: reason})
	if err != nil {
		s.log.Error("create warning", "err", err)
		return
	}
	sess, err = s.q.IncrementSessionWarnings(ctx, d.SessionID)
	if err != nil {
		s.log.Error("increment warnings", "err", err)
	}
	s.hub.Broadcast(d.SessionID, ws.TypeDeadlineUpdated, dto.FromDeadline(d))
	s.hub.Broadcast(d.SessionID, ws.TypeWarningNew, map[string]any{"warning": dto.FromWarning(w), "totalWarnings": int(sess.Warnings)})
	s.hub.Broadcast(d.SessionID, ws.TypeSessionUpdated, dto.FromSession(sess))
	msgID := d.MessageID
	if _, err := s.chat.PostSystem(ctx, d.SessionID, d.ChannelID,
		fmt.Sprintf(":warning: **Ghosting warning #%d** — a colleague has been waiting more than %s for your reply. Unanswered messages hurt your score.", sess.Warnings, humanDuration(s.timeout)),
		&msgID); err != nil {
		s.log.Error("post warning message", "err", err)
	}
	s.log.Info("deadline expired", "deadline", d.ID, "session", d.SessionID, "warnings", sess.Warnings)
}

// candidates returns the pending deadlines a user message may resolve. A reply
// resolves a deadline if it is in the same channel (top-level or in the thread
// of the requesting message).
func (s *Service) candidates(ctx context.Context, m store.Message) []store.Deadline {
	pending, err := s.q.ListPendingDeadlinesForChannel(ctx, m.ChannelID)
	if err != nil || len(pending) == 0 {
		return nil
	}
	var out []store.Deadline
	for _, d := range pending {
		if d.CreatedAt.After(m.CreatedAt) {
			continue
		}
		// Thread reply to another message: only counts for that thread's deadline,
		// or for deadlines on top-level messages if the thread parent is unrelated.
		if m.ParentID != nil && *m.ParentID != d.MessageID {
			req, err := s.q.GetMessage(ctx, d.MessageID)
			if err == nil && req.ParentID != nil && *req.ParentID != *m.ParentID {
				continue // request lives in a different thread
			}
		}
		out = append(out, d)
	}
	return out
}

// OnUserMessage snapshots pending deadlines in the message's channel and
// validates the reply against each in the background.
func (s *Service) OnUserMessage(ctx context.Context, m store.Message) {
	candidates := s.candidates(ctx, m)
	if len(candidates) == 0 {
		return
	}
	sctx, ok := s.reg.Get(m.SessionID)
	if !ok {
		return
	}
	go s.validateAndResolve(sctx, m, candidates)
}

// ValidateReply is the synchronous variant of OnUserMessage: it validates the
// reply against the pending deadlines and returns when done.
func (s *Service) ValidateReply(ctx context.Context, m store.Message) {
	if candidates := s.candidates(ctx, m); len(candidates) > 0 {
		s.validateAndResolve(ctx, m, candidates)
	}
}

func (s *Service) validateAndResolve(ctx context.Context, m store.Message, candidates []store.Deadline) {
	var lastReason string
	resolvedAny := false
	for _, d := range candidates {
		req, err := s.q.GetMessage(ctx, d.MessageID)
		if err != nil {
			continue
		}
		res := s.val.Validate(ctx, req.Content, m.Content)
		if !res.Valid {
			lastReason = res.Reason
			s.log.Info("reply did not resolve deadline", "deadline", d.ID, "reason", res.Reason)
			continue
		}
		if s.resolve(ctx, d.ID) {
			resolvedAny = true
		}
	}
	if !resolvedAny && lastReason != "" && ctx.Err() == nil {
		parent := m.ID
		if m.ParentID != nil {
			parent = *m.ParentID
		}
		_, _ = s.chat.PostSystem(ctx, m.SessionID, m.ChannelID,
			"That reply didn't count as a response to the open request: "+lastReason+". Please reply clearly in English before the deadline.", &parent)
	}
}

func (s *Service) resolve(ctx context.Context, id uuid.UUID) bool {
	d, err := s.q.ResolveDeadline(ctx, id)
	if err != nil {
		return false
	}
	s.unschedule(id)
	s.hub.Broadcast(d.SessionID, ws.TypeDeadlineUpdated, dto.FromDeadline(d))
	if d.TaskID != nil {
		// Only OPEN -> ACKNOWLEDGED; never downgrades a DONE task (no row = no change).
		if t, err := s.q.AcknowledgeTask(ctx, *d.TaskID); err == nil {
			s.hub.Broadcast(d.SessionID, ws.TypeTaskUpdated, dto.LoadTask(ctx, s.q, t))
		}
	}
	s.log.Info("deadline resolved", "deadline", d.ID)
	return true
}

func humanDuration(d time.Duration) string {
	if d >= time.Minute && d%time.Minute == 0 {
		n := int(d / time.Minute)
		if n == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", n)
	}
	return d.Round(time.Second).String()
}
