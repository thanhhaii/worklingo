// Package evaluation scores a completed session with the evaluation model.
package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

type Service struct {
	q      *store.Queries
	cat    *persona.Catalog
	hub    ws.Broadcaster
	llm    llm.Provider
	model  string
	log    *slog.Logger
	root   context.Context
	active sync.Map
}

func New(root context.Context, q *store.Queries, cat *persona.Catalog, hub ws.Broadcaster, p llm.Provider, model string, log *slog.Logger) *Service {
	return &Service{root: root, q: q, cat: cat, hub: hub, llm: p, model: model, log: log}
}

// Start creates a PENDING evaluation and runs scoring in the background.
func (s *Service) Start(sessionID uuid.UUID) {
	ctx, cancel := context.WithTimeout(s.root, 10*time.Second)
	e, err := s.q.UpsertPendingEvaluation(ctx, sessionID)
	cancel()
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		s.log.Error("create evaluation", "session", sessionID, "err", err)
		return
	}
	s.hub.Broadcast(sessionID, ws.TypeEvaluationUpdated, dto.FromEvaluation(e))
	s.launch(sessionID)
}

// Recover restarts evaluations left PENDING by a previous process.
func (s *Service) Recover(ctx context.Context) {
	es, err := s.q.ListPendingEvaluations(ctx)
	if err != nil {
		s.log.Error("list pending evaluations", "err", err)
		return
	}
	for _, e := range es {
		s.launch(e.SessionID)
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

// Retry atomically claims only unfinished work before dispatching a worker.
// Ownership is checked by the HTTP handler; the database also excludes active sessions.
func (s *Service) Retry(ctx context.Context, sessionID uuid.UUID) (dto.Evaluation, error) {
	if _, running := s.active.Load(sessionID); running {
		return dto.Evaluation{}, pgx.ErrNoRows
	}
	e, err := s.q.ClaimEvaluationRetry(ctx, sessionID)
	if err != nil {
		return dto.Evaluation{}, err
	}
	out := dto.FromEvaluation(e)
	s.hub.Broadcast(sessionID, ws.TypeEvaluationUpdated, out)
	s.launch(sessionID)
	return out, nil
}

// Reserve before dispatch so startup recovery cannot race a newly started worker.
func (s *Service) launch(sessionID uuid.UUID) {
	if _, running := s.active.LoadOrStore(sessionID, true); running {
		return
	}
	go func() {
		defer s.active.Delete(sessionID)
		s.run(sessionID)
	}()
}

func (s *Service) run(sessionID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("evaluation panic", "session", sessionID)
			s.finalize(sessionID)
		}
	}()
	ctx, cancel := context.WithTimeout(s.root, 5*time.Minute)
	defer cancel()
	existing, err := s.q.GetEvaluation(ctx, sessionID)
	if err != nil {
		s.log.Error("load evaluation", "err", err)
		s.finalize(sessionID)
		return
	}
	if existing.Status != "PENDING" {
		return
	}
	data, err := s.load(ctx, sessionID)
	if err != nil {
		s.log.Error("evaluation transcript", "err", err)
		s.finalize(sessionID)
		return
	}
	prior := dto.FromEvaluation(existing)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		if existing.ScoreStatus == "AVAILABLE" {
			return
		}
		var out result
		var err error
		if len(data.learner) == 0 {
			out = result{Grammar: f(0), Tone: f(0), Vocabulary: f(0), Overall: f(0), Summary: "You didn't send any messages in this session, so there is nothing to evaluate yet. Next time, try replying to your colleagues' questions and tasks."}
		} else {
			out, err = s.score(ctx, data)
		}
		if err != nil {
			s.log.Warn("evaluation scores unavailable", "session", sessionID)
			return
		}
		s.saveScores(sessionID, out)
	}()
	go func() {
		defer wg.Done()
		if existing.MessageReviewStatus == "AVAILABLE" {
			return
		}
		reviews := s.retryMessageReviews(ctx, data, prior.MessageReviews)
		raw, err := json.Marshal(reviews)
		if err != nil {
			s.log.Error("encode message reviews", "err", err)
			return
		}
		status := "AVAILABLE"
		for _, r := range reviews {
			if r.ReviewStatus != "AVAILABLE" {
				status = "UNAVAILABLE"
			}
		}
		s.checkpoint(sessionID, func(c context.Context) error {
			return s.q.SaveEvaluationMessageReviews(c, store.SaveEvaluationMessageReviewsParams{SessionID: sessionID, MessageReviews: raw, MessageReviewStatus: status})
		})
	}()
	go func() {
		defer wg.Done()
		if existing.TaskReviewStatus == "AVAILABLE" {
			return
		}
		reviews := s.retryTaskReviews(ctx, data, prior.TaskReviews)
		raw, err := json.Marshal(reviews)
		if err != nil {
			s.log.Error("encode task reviews", "err", err)
			return
		}
		status := "AVAILABLE"
		for _, r := range reviews {
			if r.ReviewStatus != "AVAILABLE" {
				status = "UNAVAILABLE"
			}
		}
		s.checkpoint(sessionID, func(c context.Context) error {
			return s.q.SaveEvaluationTaskReviews(c, store.SaveEvaluationTaskReviewsParams{SessionID: sessionID, TaskReviews: raw, TaskReviewStatus: status})
		})
	}()
	wg.Wait()
	s.finalize(sessionID)
}

// Each component is saved independently, including when another component fails.
func (s *Service) checkpoint(sessionID uuid.UUID, save func(context.Context) error) {
	ctx, cancel := context.WithTimeout(s.root, 10*time.Second)
	defer cancel()
	if err := save(ctx); err != nil {
		s.log.Error("save evaluation component", "session", sessionID, "err", err)
	}
}

func (s *Service) retryMessageReviews(ctx context.Context, data *sessionData, prior []dto.MessageReview) []dto.MessageReview {
	available := map[string]dto.MessageReview{}
	for _, r := range prior {
		if r.ReviewStatus == "AVAILABLE" {
			available[r.MessageID] = r
		}
	}
	pending := *data
	pending.learner = nil
	for _, m := range data.learner {
		if _, ok := available[m.ID.String()]; !ok {
			pending.learner = append(pending.learner, m)
		}
	}
	for _, r := range s.reviewMessages(ctx, &pending) {
		available[r.MessageID] = r
	}
	out := make([]dto.MessageReview, 0, len(data.learner))
	for _, m := range data.learner {
		out = append(out, available[m.ID.String()])
	}
	return out
}

func (s *Service) retryTaskReviews(ctx context.Context, data *sessionData, prior []dto.TaskReview) []dto.TaskReview {
	available := map[string]dto.TaskReview{}
	for _, r := range prior {
		if r.ReviewStatus == "AVAILABLE" {
			available[r.TaskID] = r
		}
	}
	pending := *data
	pending.tasks = nil
	for _, t := range data.tasks {
		if _, ok := available[t.ID.String()]; !ok {
			pending.tasks = append(pending.tasks, t)
		}
	}
	for _, r := range s.reviewTasks(ctx, &pending, len(data.learner) == 0) {
		available[r.TaskID] = r
	}
	out := make([]dto.TaskReview, 0, len(data.tasks))
	for _, t := range data.tasks {
		out = append(out, available[t.ID.String()])
	}
	return out
}

func (s *Service) score(ctx context.Context, d *sessionData) (result, error) {
	prompt := fmt.Sprintf("Ghosting warnings (requests left unanswered too long): %d\n\nSession transcript:\n%s", d.warnings, d.transcript)
	var out result
	temp := 0.2
	err := llm.CompleteJSON(ctx, s.llm, llm.Request{
		Model: s.model, MaxTokens: 3000, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt}, {Role: llm.RoleUser, Content: prompt}},
	}, &out)
	if err != nil {
		return out, err
	}
	if out.Grammar == nil || out.Tone == nil || out.Vocabulary == nil || strings.TrimSpace(out.Summary) == "" {
		return out, errors.New("missing scores or summary in evaluation JSON")
	}
	return out, nil
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

func (s *Service) saveScores(sessionID uuid.UUID, r result) {
	g, t, v := clamp(r.Grammar), clamp(r.Tone), clamp(r.Vocabulary)
	o := clamp(r.Overall)
	if r.Overall == nil {
		o = int32(float64(g)*0.35 + float64(t)*0.4 + float64(v)*0.25 + 0.5)
	}
	s.checkpoint(sessionID, func(ctx context.Context) error {
		return s.q.SaveEvaluationScores(ctx, store.SaveEvaluationScoresParams{
			SessionID: sessionID, Grammar: g, Tone: t, Vocabulary: v, Overall: o, Summary: strings.TrimSpace(r.Summary),
		})
	})
}

func (s *Service) finalize(sessionID uuid.UUID) {
	ctx, cancel := context.WithTimeout(s.root, 10*time.Second)
	defer cancel()
	e, err := s.q.FinalizeEvaluation(ctx, sessionID)
	if err != nil {
		s.log.Error("finalize evaluation", "session", sessionID, "err", err)
		return
	}
	s.hub.Broadcast(sessionID, ws.TypeEvaluationUpdated, dto.FromEvaluation(e))
	s.log.Info("evaluation finished", "session", sessionID, "status", e.Status)
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
