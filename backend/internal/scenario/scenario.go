// Package scenario generates surprise tasks dropped by personas during a session.
package scenario

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

type Config struct {
	FirstDelay  time.Duration
	MinInterval time.Duration
	MaxInterval time.Duration
}

type Scheduler struct {
	cfg       Config
	q         *store.Queries
	cat       *persona.Catalog
	chat      *chat.Service
	deadlines *deadline.Service
	hub       ws.Broadcaster
	llm       llm.Provider
	model     string
	log       *slog.Logger

	mu      sync.Mutex
	running map[uuid.UUID]bool
}

func New(cfg Config, q *store.Queries, cat *persona.Catalog, c *chat.Service, d *deadline.Service, hub ws.Broadcaster, p llm.Provider, model string, log *slog.Logger) *Scheduler {
	return &Scheduler{cfg: cfg, q: q, cat: cat, chat: c, deadlines: d, hub: hub, llm: p, model: model, log: log, running: map[uuid.UUID]bool{}}
}

// FirstAt returns when the first task should fire for a session starting now.
func (s *Scheduler) FirstAt(start time.Time) time.Time { return start.Add(s.cfg.FirstDelay) }

func (s *Scheduler) nextInterval() time.Duration {
	span := s.cfg.MaxInterval - s.cfg.MinInterval
	if span <= 0 {
		return s.cfg.MinInterval
	}
	return s.cfg.MinInterval + time.Duration(rand.Int64N(int64(span)))
}

// Run starts the scheduler loop for a session; it stops when ctx is cancelled.
// nextAt is the persisted time of the next task (nil = FirstDelay from now).
func (s *Scheduler) Run(ctx context.Context, sessionID uuid.UUID, nextAt *time.Time) {
	s.mu.Lock()
	if s.running[sessionID] {
		s.mu.Unlock()
		return
	}
	s.running[sessionID] = true
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.running, sessionID)
			s.mu.Unlock()
			if r := recover(); r != nil {
				s.log.Error("scenario panic", "panic", r)
			}
		}()
		next := time.Now().Add(s.cfg.FirstDelay)
		if nextAt != nil {
			next = *nextAt
		}
		for {
			t := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case <-t.C:
			}
			if err := s.fire(ctx, sessionID); err != nil && ctx.Err() == nil {
				s.log.Error("surprise task failed", "session", sessionID, "err", err)
			}
			next = time.Now().Add(s.nextInterval())
			n := next
			if err := s.q.SetSessionNextTaskAt(context.WithoutCancel(ctx), store.SetSessionNextTaskAtParams{ID: sessionID, NextTaskAt: &n}); err != nil {
				s.log.Warn("persist next task time", "err", err)
			}
		}
	}()
}

type generated struct {
	PersonaID   string `json:"personaId"`
	Channel     string `json:"channel"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Message     string `json:"message"`
}

func (s *Scheduler) fire(ctx context.Context, sessionID uuid.UUID) error {
	sess, err := s.q.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != "ACTIVE" || time.Now().After(sess.ExpiresAt) {
		return nil
	}
	user, err := s.q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return err
	}
	channels, err := s.q.ListChannels(ctx, sessionID)
	if err != nil {
		return err
	}
	tasks, _ := s.q.ListTasks(ctx, sessionID)

	g, err := s.generate(ctx, user.Username, tasks)
	if err != nil {
		s.log.Warn("task generation failed, using fallback", "err", err)
		g = s.fallback(user.Username)
	}
	g = s.normalise(g, user.Username)

	var ch store.Channel
	found := false
	for _, c := range channels {
		if (g.Channel == "dm" && c.Kind == "DM" && c.PersonaID != nil && *c.PersonaID == g.PersonaID) ||
			(c.Kind == "CHANNEL" && c.Name == g.Channel) {
			ch, found = c, true
			break
		}
	}
	if !found {
		for _, c := range channels {
			if c.Kind == "CHANNEL" && c.Name == "general" {
				ch, found = c, true
			}
		}
	}
	if !found {
		return fmt.Errorf("no channel for task")
	}

	s.chat.Typing(sessionID, ch.ID, g.PersonaID)
	msg, err := s.chat.PostPersona(ctx, sessionID, ch.ID, g.PersonaID, g.Message, nil)
	if err != nil {
		return err
	}
	task, err := s.q.CreateTask(ctx, store.CreateTaskParams{
		SessionID: sessionID, ChannelID: ch.ID, MessageID: msg.ID, PersonaID: g.PersonaID,
		Title: g.Title, Description: g.Description, Priority: g.Priority,
	})
	if err != nil {
		return err
	}
	s.hub.Broadcast(sessionID, ws.TypeTaskNew, dto.FromTask(task))
	if _, err := s.deadlines.Create(ctx, sessionID, ch.ID, msg.ID, &task.ID); err != nil {
		return err
	}
	s.log.Info("surprise task created", "session", sessionID, "task", task.ID, "persona", g.PersonaID, "channel", ch.Name)
	return nil
}

func (s *Scheduler) generate(ctx context.Context, username string, prev []store.Task) (generated, error) {
	pid := persona.Order[rand.IntN(len(persona.Order))]
	p, _ := s.cat.Get(pid)
	var prevTitles []string
	for _, t := range prev {
		prevTitles = append(prevTitles, t.Title)
	}
	prompt := fmt.Sprintf(`You design surprise workplace tasks for a Business English Slack simulator at a software company.
The learner is @%s, a software engineer.
Create ONE new task from %s (@%s, %s).
%s
Ideas for this persona (pick one or invent a similar one): %s
Avoid repeating previous tasks: %s
Channels: "general", "sprint-planning", "incident-room", "watercooler", or "dm" (private DM from the persona). Incidents go to "incident-room", planning to "sprint-planning".
"message" is the Slack message the persona posts, written fully in character (1-4 sentences), it MUST @mention @%s and end with a direct question or request that needs a written reply.
"priority" is "CRITICAL" only for urgent production issues or angry clients, otherwise "NORMAL".
Respond with ONLY JSON:
{"personaId":"%s","channel":"...","title":"short task title (max 8 words)","description":"one sentence describing what the learner must do","priority":"NORMAL","message":"..."}`,
		username, p.DisplayName, p.Handle, p.Role, strings.TrimSpace(p.SystemPrompt), strings.Join(p.TaskIdeas, "; "),
		orNone(prevTitles), username, pid)
	var g generated
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	err := llm.CompleteJSON(cctx, s.llm, llm.Request{Model: s.model, MaxTokens: 500,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}}}, &g)
	if err != nil {
		return g, err
	}
	if g.PersonaID == "" {
		g.PersonaID = pid
	}
	if strings.TrimSpace(g.Message) == "" || strings.TrimSpace(g.Title) == "" {
		return g, fmt.Errorf("incomplete task JSON")
	}
	return g, nil
}

func orNone(xs []string) string {
	if len(xs) == 0 {
		return "(none)"
	}
	return strings.Join(xs, "; ")
}

func (s *Scheduler) normalise(g generated, username string) generated {
	if _, ok := s.cat.Get(g.PersonaID); !ok {
		g.PersonaID = "sarah"
	}
	g.Channel = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(g.Channel)), "#")
	switch g.Channel {
	case "general", "sprint-planning", "incident-room", "watercooler", "dm":
	default:
		g.Channel = "general"
	}
	g.Priority = strings.ToUpper(strings.TrimSpace(g.Priority))
	if g.Priority != "CRITICAL" {
		g.Priority = "NORMAL"
	}
	g.Message = strings.TrimSpace(g.Message)
	if !chat.Mentions(g.Message, username) {
		g.Message = "@" + username + " " + g.Message
	}
	if g.Description == "" {
		g.Description = g.Title
	}
	return g
}

var fallbacks = []generated{
	{PersonaID: "sarah", Channel: "sprint-planning", Title: "Estimate a new user story", Description: "Give Sarah an estimate for the new export story.", Priority: "NORMAL",
		Message: "Stakeholders just asked for a CSV export on the reports page. @{{u}} can you give me a rough estimate in story points before standup?"},
	{PersonaID: "alex", Channel: "incident-room", Title: "Explain the 500 errors", Description: "Tell Alex what you know about the spike of 500 errors on the API.", Priority: "CRITICAL",
		Message: ":rotating_light: We're seeing a spike of 500s on `/api/orders`. @{{u}} you touched that handler yesterday — what changed, and can you check the logs?"},
	{PersonaID: "david", Channel: "dm", Title: "Clarify the client's request", Description: "Ask David clarifying questions about his new homepage idea.", Priority: "NORMAL",
		Message: "Hello! Had a brilliant chat with our marketing lot — we'd like the homepage to feel a bit more 'premium'. @{{u}} could you let me know what that would involve, ballpark?"},
}

func (s *Scheduler) fallback(username string) generated {
	g := fallbacks[rand.IntN(len(fallbacks))]
	g.Message = strings.ReplaceAll(g.Message, "{{u}}", username)
	return g
}
