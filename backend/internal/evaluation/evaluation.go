// Package evaluation scores a completed session with the evaluation model.
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

type Service struct {
	q     *store.Queries
	cat   *persona.Catalog
	hub   ws.Broadcaster
	llm   llm.Provider
	model string
	log   *slog.Logger
	root  context.Context
}

func New(root context.Context, q *store.Queries, cat *persona.Catalog, hub ws.Broadcaster, p llm.Provider, model string, log *slog.Logger) *Service {
	return &Service{root: root, q: q, cat: cat, hub: hub, llm: p, model: model, log: log}
}

// Start creates a PENDING evaluation and runs scoring in the background.
func (s *Service) Start(sessionID uuid.UUID) {
	ctx, cancel := context.WithTimeout(s.root, 10*time.Second)
	e, err := s.q.UpsertPendingEvaluation(ctx, sessionID)
	cancel()
	if err != nil {
		s.log.Error("create evaluation", "session", sessionID, "err", err)
		return
	}
	s.hub.Broadcast(sessionID, ws.TypeEvaluationUpdated, dto.FromEvaluation(e))
	go s.run(sessionID)
}

// Recover restarts evaluations left PENDING by a previous process.
func (s *Service) Recover(ctx context.Context) {
	es, err := s.q.ListPendingEvaluations(ctx)
	if err != nil {
		s.log.Error("list pending evaluations", "err", err)
		return
	}
	for _, e := range es {
		go s.run(e.SessionID)
	}
}

type result struct {
	Grammar    *float64 `json:"grammar"`
	Tone       *float64 `json:"tone"`
	Vocabulary *float64 `json:"vocabulary"`
	Overall    *float64 `json:"overall"`
	Summary    string   `json:"summary"`
}

const systemPrompt = `You are an expert Business English coach. You evaluate a learner's written messages from a simulated workplace Slack session at a software company (colleagues: a demanding product manager, a strict tech lead, and a British client).
Score ONLY the learner's own messages (marked LEARNER), using the other messages as context.

Rubric (each 0-100):
- grammar: accuracy of grammar, spelling, punctuation, sentence structure.
- tone: professional, polite, clear and appropriately direct for Slack; handles pushback, commitments, and clients well; responsiveness (missed replies / ghosting warnings lower this).
- vocabulary: range and precision of business/technical vocabulary, natural collocations and idioms.
- overall: holistic score, roughly the weighted average (grammar 35%, tone 40%, vocabulary 25%) adjusted for responsiveness.
Calibration: 90+ near-native professional; 70-89 good with minor issues; 50-69 understandable but frequent errors; below 50 hard to understand. If the learner wrote almost nothing, give low scores and say so.

summary: 2-4 sentences of encouraging, specific feedback, addressed to the learner as "you".

Respond with ONLY a JSON object, no markdown:
{"grammar": 0, "tone": 0, "vocabulary": 0, "overall": 0, "summary": "..."}`

func (s *Service) run(sessionID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("evaluation panic", "panic", r)
			s.fail(sessionID, "Internal error while evaluating.")
		}
	}()
	ctx, cancel := context.WithTimeout(s.root, 5*time.Minute)
	defer cancel()

	data, err := s.load(ctx, sessionID)
	if err != nil {
		s.log.Error("evaluation transcript", "err", err)
		s.fail(sessionID, "Could not load the session transcript.")
		return
	}
	if len(data.learner) == 0 {
		s.complete(ctx, sessionID, result{
			Grammar: f(0), Tone: f(0), Vocabulary: f(0), Overall: f(0),
			Summary: "You didn't send any messages in this session, so there is nothing to evaluate yet. Next time, try replying to your colleagues' questions and tasks.",
		}, nil, s.reviewTasks(ctx, data, true))
		return
	}

	// Scoring and the two reviews run concurrently; only scoring failure fails the evaluation.
	var (
		wg      sync.WaitGroup
		out     result
		scoreEr error
		mreview []dto.MessageReview
		treview []dto.TaskReview
	)
	wg.Add(3)
	go func() { defer wg.Done(); out, scoreEr = s.score(ctx, data) }()
	go func() { defer wg.Done(); mreview = s.reviewMessages(ctx, data) }()
	go func() { defer wg.Done(); treview = s.reviewTasks(ctx, data, false) }()
	wg.Wait()
	if scoreEr != nil {
		s.fail(sessionID, "The evaluation service is unavailable right now.")
		return
	}
	s.complete(ctx, sessionID, out, mreview, treview)
}

func (s *Service) score(ctx context.Context, d *sessionData) (result, error) {
	prompt := fmt.Sprintf("Ghosting warnings (requests left unanswered too long): %d\n\nSession transcript:\n%s", d.warnings, d.transcript)
	var out result
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		temp := 0.2
		lastErr = llm.CompleteJSON(ctx, s.llm, llm.Request{
			Model: s.model, MaxTokens: 3000, Temperature: &temp,
			Messages: []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt}, {Role: llm.RoleUser, Content: prompt}},
		}, &out)
		if lastErr == nil && out.Grammar != nil && out.Tone != nil && out.Vocabulary != nil {
			return out, nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("missing scores in evaluation JSON")
		}
		s.log.Warn("evaluation attempt failed", "attempt", attempt+1, "err", lastErr)
	}
	return out, lastErr
}

func f(v float64) *float64 { return &v }

func clamp(p *float64) int32 {
	if p == nil {
		return 0
	}
	v := int32(*p + 0.5)
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func (s *Service) complete(ctx context.Context, sessionID uuid.UUID, r result, mreview []dto.MessageReview, treview []dto.TaskReview) {
	g, t, v := clamp(r.Grammar), clamp(r.Tone), clamp(r.Vocabulary)
	o := clamp(r.Overall)
	if r.Overall == nil {
		o = int32(float64(g)*0.35 + float64(t)*0.4 + float64(v)*0.25 + 0.5)
	}
	if mreview == nil {
		mreview = []dto.MessageReview{}
	}
	if treview == nil {
		treview = []dto.TaskReview{}
	}
	mraw, _ := json.Marshal(mreview)
	traw, _ := json.Marshal(treview)
	e, err := s.q.CompleteEvaluation(ctx, store.CompleteEvaluationParams{
		SessionID: sessionID, Grammar: g, Tone: t, Vocabulary: v, Overall: o,
		Summary: strings.TrimSpace(r.Summary), MessageReviews: mraw, TaskReviews: traw,
	})
	if err != nil {
		s.log.Error("store evaluation", "err", err)
		s.fail(sessionID, "Could not save the evaluation.")
		return
	}
	s.hub.Broadcast(sessionID, ws.TypeEvaluationUpdated, dto.FromEvaluation(e))
	s.log.Info("evaluation done", "session", sessionID, "overall", o)
}

func (s *Service) fail(sessionID uuid.UUID, reason string) {
	ctx, cancel := context.WithTimeout(s.root, 10*time.Second)
	defer cancel()
	e, err := s.q.FailEvaluation(ctx, store.FailEvaluationParams{SessionID: sessionID, Summary: reason})
	if err != nil {
		s.log.Error("fail evaluation", "err", err)
		return
	}
	s.hub.Broadcast(sessionID, ws.TypeEvaluationUpdated, dto.FromEvaluation(e))
}

// sessionData is everything the evaluation calls need, loaded once.
type sessionData struct {
	transcript string
	warnings   int
	msgs       []store.Message // non-system, non-empty, chronological
	learner    []store.Message // USER-authored subset
	labels     map[uuid.UUID]string
	tasks      []store.Task
	points     map[uuid.UUID][]store.TalkingPoint
}

func (s *Service) load(ctx context.Context, sessionID uuid.UUID) (*sessionData, error) {
	sess, err := s.q.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	channels, err := s.q.ListChannels(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	all, err := s.q.ListSessionMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.q.ListTasks(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	pts, err := s.q.ListTalkingPointsForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	d := &sessionData{warnings: int(sess.Warnings), tasks: tasks, points: map[uuid.UUID][]store.TalkingPoint{}}
	for _, p := range pts {
		d.points[p.TaskID] = append(d.points[p.TaskID], p)
	}
	d.labels = channelLabels(channels, func(id string) (string, bool) {
		p, ok := s.cat.Get(id)
		return p.DisplayName, ok
	})
	var sb strings.Builder
	const maxChars = 60000
	full := false
	for _, m := range all {
		if strings.TrimSpace(m.Content) == "" || m.AuthorType == chat.AuthorSystem {
			continue
		}
		d.msgs = append(d.msgs, m)
		who := "LEARNER"
		if m.AuthorType == chat.AuthorPersona && m.PersonaID != nil {
			if p, ok := s.cat.Get(*m.PersonaID); ok {
				who = fmt.Sprintf("%s (%s)", p.DisplayName, p.Role)
			}
		}
		if m.AuthorType == chat.AuthorUser {
			d.learner = append(d.learner, m)
		}
		line := fmt.Sprintf("[%s] %s: %s\n", d.labels[m.ChannelID], who, m.Content)
		if !full && sb.Len()+len(line) > maxChars {
			full = true
		}
		if !full {
			sb.WriteString(line)
		}
	}
	d.transcript = sb.String()
	return d, nil
}
