package scenario

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/focus"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

func (s *Scheduler) runFocused(ctx context.Context, id uuid.UUID) {
	for {
		sess, err := s.q.GetSession(ctx, id)
		if err != nil || sess.Status != "ACTIVE" || sess.NextTaskAt == nil || !time.Now().Before(sess.ExpiresAt) {
			return
		}
		timer := time.NewTimer(time.Until(*sess.NextTaskAt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err = s.fireFocused(ctx, id); err != nil {
			if ctx.Err() == nil {
				s.log.Error("focused task failed", "err", err)
			}
			return
		}
	}
}

// The row lock makes task insertion, progress and schedule advancement atomic
// across restarts and concurrent scheduler instances. Events follow the commit.
func (s *Scheduler) fireFocused(ctx context.Context, id uuid.UUID) error {
	var msg store.Message
	var task store.Task
	var points []store.TalkingPoint
	inserted := false
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		sess, err := q.LockSession(ctx, id)
		if err != nil {
			return err
		}
		now := time.Now()
		if sess.LearningGoal == nil || sess.Status != "ACTIVE" || !now.Before(sess.ExpiresAt) || sess.NextTaskAt == nil || now.Before(*sess.NextTaskAt) {
			return nil
		}
		limit := focus.TaskLimit(sess.ExpiresAt.Sub(sess.StartedAt))
		step := int(sess.FocusedStep)
		if step >= limit {
			return nil
		}
		templates := focus.Templates(*sess.LearningGoal)
		if step >= len(templates) {
			return fmt.Errorf("invalid focused step")
		}
		tpl := templates[step]
		user, err := q.GetUserByID(ctx, sess.UserID)
		if err != nil {
			return err
		}
		channels, err := q.ListChannels(ctx, id)
		if err != nil {
			return err
		}
		var ch store.Channel
		for _, c := range channels {
			if c.Kind == "CHANNEL" && c.Name == tpl.Channel {
				ch = c
				break
			}
		}
		if ch.ID == uuid.Nil {
			return fmt.Errorf("missing focused channel %s", tpl.Channel)
		}
		pid := tpl.PersonaID
		msg, err = q.CreateMessage(ctx, store.CreateMessageParams{SessionID: id, ChannelID: ch.ID, AuthorType: "PERSONA", PersonaID: &pid, Content: persona.Render(tpl.Message, user.Username)})
		if err != nil {
			return err
		}
		task, err = q.CreateTask(ctx, store.CreateTaskParams{SessionID: id, ChannelID: ch.ID, MessageID: msg.ID, PersonaID: pid, Title: tpl.Title, Priority: "NORMAL"})
		if err != nil {
			return err
		}
		for i, p := range tpl.Points {
			pt, err := q.CreateTalkingPoint(ctx, store.CreateTalkingPointParams{TaskID: task.ID, Position: int32(i), Content: p})
			if err != nil {
				return err
			}
			points = append(points, pt)
		}
		var next *time.Time
		if step+1 < limit {
			due := focus.Due(sess.StartedAt, sess.ExpiresAt, step+1)
			if due.Before(now.Add(1500 * time.Millisecond)) {
				due = now.Add(1500 * time.Millisecond)
			}
			next = &due
		}
		if err = q.AdvanceFocusedSession(ctx, store.AdvanceFocusedSessionParams{ID: id, NextTaskAt: next}); err != nil {
			return err
		}
		inserted = true
		return nil
	})
	if err == nil && inserted {
		s.hub.Broadcast(id, ws.TypeMessageNew, dto.FromMessage(msg))
		s.hub.Broadcast(id, ws.TypeTaskNew, dto.FromTask(task, points))
	}
	return err
}
