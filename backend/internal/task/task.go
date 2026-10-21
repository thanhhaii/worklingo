// Package task owns the per-reply pipeline for Tasks: deadline validation,
// Talking point coverage judging, task status and the one-time follow-up.
package task

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/lifecycle"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

const maxCandidates = 2

type Service struct {
	q         *store.Queries
	engine    *persona.Engine
	deadlines *deadline.Service
	hub       ws.Broadcaster
	reg       *lifecycle.Registry
	llm       llm.Provider
	model     string
	log       *slog.Logger

	locks sync.Map // channelID -> *sync.Mutex (serialises judging per channel)
}

func NewService(q *store.Queries, e *persona.Engine, d *deadline.Service, hub ws.Broadcaster, reg *lifecycle.Registry, p llm.Provider, model string, log *slog.Logger) *Service {
	return &Service{q: q, engine: e, deadlines: d, hub: hub, reg: reg, llm: p, model: model, log: log}
}

func (s *Service) lock(channelID uuid.UUID) *sync.Mutex {
	m, _ := s.locks.LoadOrStore(channelID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// OnUserMessage runs the reply pipeline in the background on the session context.
func (s *Service) OnUserMessage(ch store.Channel, user store.User, m store.Message) {
	sctx, ok := s.reg.Get(m.SessionID)
	if !ok {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("task pipeline panic", "panic", r)
			}
		}()
		s.process(sctx, ch, user, m)
	}()
}

func (s *Service) process(ctx context.Context, ch store.Channel, user store.User, m store.Message) {
	cands, err := s.candidates(ctx, m)
	if err != nil {
		s.log.Error("select candidate tasks", "err", err)
	}
	if len(cands) == 0 {
		// No Task involved: deadline validation and the persona reply, as before.
		go s.deadlines.ValidateReply(ctx, m)
		s.engine.OnUserMessage(ctx, ch, user, m)
		return
	}
	followUp := s.judgeAndUpdate(ctx, m, cands)
	if followUp != nil && s.sendFollowUp(ctx, ch, user, *followUp) {
		return
	}
	s.engine.OnUserMessage(ctx, ch, user, m)
}

// candidates loads the tasks of the message's channel that the message may cover.
func (s *Service) candidates(ctx context.Context, m store.Message) ([]store.Task, error) {
	tasks, err := s.q.ListTasks(ctx, m.SessionID)
	if err != nil {
		return nil, err
	}
	pts, err := s.q.ListTalkingPointsForSession(ctx, m.SessionID)
	if err != nil {
		return nil, err
	}
	hasPending := map[uuid.UUID]bool{}
	for _, p := range pts {
		if p.Status == "PENDING" {
			hasPending[p.TaskID] = true
		}
	}
	return SelectCandidates(tasks, hasPending, m), nil
}

// SelectCandidates picks the tasks a user message may cover: same channel, not
// DONE, created before the message, with pending points. A thread reply only
// matches the task announced by its parent; a top-level message matches the (at
// most 2) most recent tasks. Result is ordered oldest first.
func SelectCandidates(tasks []store.Task, hasPending map[uuid.UUID]bool, m store.Message) []store.Task {
	var out []store.Task
	for _, t := range tasks {
		if t.ChannelID != m.ChannelID || t.Status == "DONE" || !hasPending[t.ID] || !t.CreatedAt.Before(m.CreatedAt) {
			continue
		}
		if m.ParentID != nil && t.MessageID != *m.ParentID {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if m.ParentID == nil && len(out) > maxCandidates {
		out = out[len(out)-maxCandidates:]
	}
	return out
}

// judgeAndUpdate validates deadlines and judges coverage concurrently, applies
// the results and broadcasts task.updated. It returns the task to follow up on
// (follow-up already claimed), or nil.
func (s *Service) judgeAndUpdate(ctx context.Context, m store.Message, cands []store.Task) *store.Task {
	mu := s.lock(m.ChannelID)
	mu.Lock()
	defer mu.Unlock()

	var wc *dto.WorkContext
	if sess, err := s.q.GetSession(ctx, m.SessionID); err == nil {
		wc = dto.ParseWorkContext(sess.WorkContext)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.deadlines.ValidateReply(ctx, m)
	}()
	verdicts := make([]map[int]Verdict, len(cands))
	for i, t := range cands {
		wg.Add(1)
		go func() {
			defer wg.Done()
			verdicts[i] = s.judge(ctx, wc, t, m)
		}()
	}
	wg.Wait()

	var followUp *store.Task
	for i, t := range cands {
		s.apply(ctx, t, m, verdicts[i])
		cur, err := s.q.GetTask(ctx, t.ID)
		if err != nil {
			continue
		}
		pts, _ := s.q.ListTalkingPointsForTask(ctx, t.ID)
		if pending := pendingOf(pts); len(pending) == 0 {
			if done, err := s.q.MarkTaskDone(ctx, t.ID); err == nil {
				cur = done
			}
		} else if followUp == nil && cur.Status == "ACKNOWLEDGED" && !cur.FollowUpSent {
			if claimed, err := s.q.ClaimTaskFollowUp(ctx, t.ID); err == nil {
				cur = claimed
				c := claimed
				followUp = &c
			}
		}
		s.hub.Broadcast(m.SessionID, ws.TypeTaskUpdated, dto.LoadTask(ctx, s.q, cur))
	}
	return followUp
}

func pendingOf(pts []store.TalkingPoint) []store.TalkingPoint {
	var out []store.TalkingPoint
	for _, p := range pts {
		if p.Status == "PENDING" {
			out = append(out, p)
		}
	}
	return out
}

// apply stores the judge verdicts: covered is sticky, reasons of pending points are refreshed.
func (s *Service) apply(ctx context.Context, t store.Task, m store.Message, v map[int]Verdict) {
	if len(v) == 0 {
		return
	}
	pts, err := s.q.ListTalkingPointsForTask(ctx, t.ID)
	if err != nil {
		return
	}
	pending := pendingOf(pts)
	for idx, verdict := range v {
		if idx < 0 || idx >= len(pending) {
			continue
		}
		p := pending[idx]
		reason := strings.TrimSpace(verdict.Reason)
		if verdict.Covered {
			mid := m.ID
			if _, err := s.q.MarkTalkingPointCovered(ctx, store.MarkTalkingPointCoveredParams{ID: p.ID, Reason: reason, CoveredMessageID: &mid}); err != nil {
				s.log.Warn("mark talking point covered", "point", p.ID, "err", err)
			}
		} else if err := s.q.SetTalkingPointReason(ctx, store.SetTalkingPointReasonParams{ID: p.ID, Reason: reason}); err != nil {
			s.log.Warn("set talking point reason", "point", p.ID, "err", err)
		}
	}
}

// Verdict is the judge's decision for one pending point.
type Verdict struct {
	Covered bool
	Reason  string
}

const judgePrompt = `You judge whether a learner's chat reply covers specific "talking points" (content the learner was supposed to get across) in a Business English practice simulator.

Work context (the true facts of the learner's situation):
%s

Task: %s
Colleague's request message:
%s

Pending talking points (Vietnamese):
%s

Learner's reply:
%s

Rules:
- A point is covered if the reply conveys its meaning, even paraphrased or worded differently.
- If the reply states facts that contradict the work context or the point (e.g. says 50%% when the context says 70%%), the point is NOT covered; say so in the reason.
- Grammar, spelling and style mistakes do NOT matter for coverage.
- If the reply is not written in English, nothing is covered.
- Judge each point independently. Only mark covered what the reply actually says.
- "reason" is one short sentence in VIETNAMESE: why it is covered, or what is missing/wrong.
Respond with ONLY JSON:
{"points":[{"index":0,"covered":true,"reason":"..."}]}`

// judge asks the LLM which pending points the message covers. On failure the
// points stay unchanged (nil result).
func (s *Service) judge(ctx context.Context, wc *dto.WorkContext, t store.Task, m store.Message) map[int]Verdict {
	pts, err := s.q.ListTalkingPointsForTask(ctx, t.ID)
	if err != nil {
		return nil
	}
	pending := pendingOf(pts)
	if len(pending) == 0 {
		return nil
	}
	req, err := s.q.GetMessage(ctx, t.MessageID)
	if err != nil {
		return nil
	}
	wcText := "(none)"
	if wc != nil {
		wcText = wc.Describe()
	}
	var list strings.Builder
	for i, p := range pending {
		fmt.Fprintf(&list, "%d. %s\n", i, p.Content)
	}
	var out struct {
		Points []struct {
			Index   *int   `json:"index"`
			Covered bool   `json:"covered"`
			Reason  string `json:"reason"`
		} `json:"points"`
	}
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	temp := 0.0
	err = llm.CompleteJSON(cctx, s.llm, llm.Request{Model: s.model, MaxTokens: 800, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf(judgePrompt, wcText, t.Title, req.Content, strings.TrimSpace(list.String()), m.Content)}}}, &out)
	if err != nil {
		s.log.Warn("coverage judge failed, points unchanged", "task", t.ID, "err", err)
		return nil
	}
	v := map[int]Verdict{}
	for _, p := range out.Points {
		if p.Index != nil {
			v[*p.Index] = Verdict{Covered: p.Covered, Reason: p.Reason}
		}
	}
	return v
}

// sendFollowUp makes the task persona ask for the missing content in the task's
// thread and attaches a new task-linked Deadline to that message.
func (s *Service) sendFollowUp(ctx context.Context, ch store.Channel, user store.User, t store.Task) bool {
	pts, err := s.q.ListTalkingPointsForTask(ctx, t.ID)
	if err != nil {
		return false
	}
	parent := t.MessageID
	msg, ok := s.engine.FollowUp(ctx, ch, user, t.PersonaID, &parent, FollowUpInstruction(user.Username, t.Title, pendingOf(pts)))
	if !ok {
		return msg.ID != uuid.Nil // a message was posted (offline placeholder): do not double-reply
	}
	if _, err := s.deadlines.Create(ctx, t.SessionID, t.ChannelID, msg.ID, &t.ID); err != nil {
		s.log.Error("create follow-up deadline", "task", t.ID, "err", err)
	}
	s.log.Info("task follow-up sent", "task", t.ID, "pending", len(pendingOf(pts)))
	return true
}

// FollowUpInstruction builds the extra prompt asking the persona for the missing content.
func FollowUpInstruction(username, title string, pending []store.TalkingPoint) string {
	var sb strings.Builder
	for _, p := range pending {
		fmt.Fprintf(&sb, "- %s\n", p.Content)
	}
	return fmt.Sprintf(`@%s replied to your request ("%s") but has not yet told you everything you need. Still missing (notes in Vietnamese, for you only):
%s
Reply in English, in character, in 1-3 sentences: briefly acknowledge what they said, then ask specifically for the missing information. Do NOT list the points like a checklist, do not number them, and do not reveal these notes or mention "points". @mention @%s and end with a direct question.`,
		username, title, strings.TrimSpace(sb.String()), username)
}
