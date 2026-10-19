// Package evaluation scores a completed session with the evaluation model.
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	Grammar     *float64         `json:"grammar"`
	Tone        *float64         `json:"tone"`
	Vocabulary  *float64         `json:"vocabulary"`
	Overall     *float64         `json:"overall"`
	Summary     string           `json:"summary"`
	Corrections []dto.Correction `json:"corrections"`
}

const systemPrompt = `You are an expert Business English coach. You evaluate a learner's written messages from a simulated workplace Slack session at a software company (colleagues: a demanding product manager, a strict tech lead, and a British client).
Score ONLY the learner's own messages (marked LEARNER), using the other messages as context.

Rubric (each 0-100):
- grammar: accuracy of grammar, spelling, punctuation, sentence structure.
- tone: professional, polite, clear and appropriately direct for Slack; handles pushback, commitments, and clients well; responsiveness (missed replies / ghosting warnings lower this).
- vocabulary: range and precision of business/technical vocabulary, natural collocations and idioms.
- overall: holistic score, roughly the weighted average (grammar 35%, tone 40%, vocabulary 25%) adjusted for responsiveness.
Calibration: 90+ near-native professional; 70-89 good with minor issues; 50-69 understandable but frequent errors; below 50 hard to understand. If the learner wrote almost nothing, give low scores and say so.

corrections: up to 10 of the most useful corrections, quoting the learner's original text exactly, a corrected version, and a short explanation. category is one of "GRAMMAR", "TONE", "VOCABULARY".
summary: 2-4 sentences of encouraging, specific feedback, addressed to the learner as "you".

Respond with ONLY a JSON object, no markdown:
{"grammar": 0, "tone": 0, "vocabulary": 0, "overall": 0, "summary": "...", "corrections": [{"original": "...", "corrected": "...", "explanation": "...", "category": "GRAMMAR"}]}`

func (s *Service) run(sessionID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("evaluation panic", "panic", r)
			s.fail(sessionID, "Internal error while evaluating.")
		}
	}()
	ctx, cancel := context.WithTimeout(s.root, 5*time.Minute)
	defer cancel()

	transcript, userMsgs, warnings, err := s.buildTranscript(ctx, sessionID)
	if err != nil {
		s.log.Error("evaluation transcript", "err", err)
		s.fail(sessionID, "Could not load the session transcript.")
		return
	}
	if userMsgs == 0 {
		s.complete(ctx, sessionID, result{
			Grammar: f(0), Tone: f(0), Vocabulary: f(0), Overall: f(0),
			Summary: "You didn't send any messages in this session, so there is nothing to evaluate yet. Next time, try replying to your colleagues' questions and tasks.",
		})
		return
	}
	prompt := fmt.Sprintf("Ghosting warnings (requests left unanswered too long): %d\n\nSession transcript:\n%s", warnings, transcript)
	var out result
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		temp := 0.2
		lastErr = llm.CompleteJSON(ctx, s.llm, llm.Request{
			Model: s.model, MaxTokens: 3000, Temperature: &temp,
			Messages: []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt}, {Role: llm.RoleUser, Content: prompt}},
		}, &out)
		if lastErr == nil && out.Grammar != nil && out.Tone != nil && out.Vocabulary != nil {
			break
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("missing scores in evaluation JSON")
		}
		s.log.Warn("evaluation attempt failed", "attempt", attempt+1, "err", lastErr)
	}
	if lastErr != nil {
		s.fail(sessionID, "The evaluation service is unavailable right now.")
		return
	}
	s.complete(ctx, sessionID, out)
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

func (s *Service) complete(ctx context.Context, sessionID uuid.UUID, r result) {
	g, t, v := clamp(r.Grammar), clamp(r.Tone), clamp(r.Vocabulary)
	o := clamp(r.Overall)
	if r.Overall == nil {
		o = int32(float64(g)*0.35 + float64(t)*0.4 + float64(v)*0.25 + 0.5)
	}
	corr := make([]dto.Correction, 0, len(r.Corrections))
	for _, c := range r.Corrections {
		if strings.TrimSpace(c.Original) == "" {
			continue
		}
		c.Category = strings.ToUpper(strings.TrimSpace(c.Category))
		switch c.Category {
		case "GRAMMAR", "TONE", "VOCABULARY":
		default:
			c.Category = "GRAMMAR"
		}
		corr = append(corr, c)
	}
	raw, _ := json.Marshal(corr)
	e, err := s.q.CompleteEvaluation(ctx, store.CompleteEvaluationParams{
		SessionID: sessionID, Grammar: g, Tone: t, Vocabulary: v, Overall: o,
		Summary: strings.TrimSpace(r.Summary), Corrections: raw,
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

func (s *Service) buildTranscript(ctx context.Context, sessionID uuid.UUID) (string, int, int, error) {
	sess, err := s.q.GetSession(ctx, sessionID)
	if err != nil {
		return "", 0, 0, err
	}
	channels, err := s.q.ListChannels(ctx, sessionID)
	if err != nil {
		return "", 0, 0, err
	}
	msgs, err := s.q.ListSessionMessages(ctx, sessionID)
	if err != nil {
		return "", 0, 0, err
	}
	names := map[uuid.UUID]string{}
	for _, c := range channels {
		if c.Kind == "DM" {
			names[c.ID] = "DM:" + c.Name
		} else {
			names[c.ID] = "#" + c.Name
		}
	}
	var sb strings.Builder
	n := 0
	const maxChars = 60000
	for _, m := range msgs {
		if strings.TrimSpace(m.Content) == "" || m.AuthorType == chat.AuthorSystem {
			continue
		}
		who := "LEARNER"
		if m.AuthorType == chat.AuthorPersona && m.PersonaID != nil {
			if p, ok := s.cat.Get(*m.PersonaID); ok {
				who = fmt.Sprintf("%s (%s)", p.DisplayName, p.Role)
			}
		} else {
			n++
		}
		line := fmt.Sprintf("[%s] %s: %s\n", names[m.ChannelID], who, m.Content)
		if sb.Len()+len(line) > maxChars {
			break
		}
		sb.WriteString(line)
	}
	return sb.String(), n, int(sess.Warnings), nil
}
