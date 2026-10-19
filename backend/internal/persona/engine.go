package persona

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
	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/store"
)

const (
	historySize   = 30
	maxResponders = 2
)

type Engine struct {
	cat       *Catalog
	q         *store.Queries
	chat      *chat.Service
	deadlines *deadline.Service
	llm       llm.Provider
	model     string
	log       *slog.Logger

	locks sync.Map // channelID -> *sync.Mutex (serialises persona replies per channel)
}

func NewEngine(cat *Catalog, q *store.Queries, c *chat.Service, d *deadline.Service, p llm.Provider, model string, log *slog.Logger) *Engine {
	return &Engine{cat: cat, q: q, chat: c, deadlines: d, llm: p, model: model, log: log}
}

func (e *Engine) Catalog() *Catalog { return e.cat }

func (e *Engine) lock(channelID uuid.UUID) *sync.Mutex {
	m, _ := e.locks.LoadOrStore(channelID, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// ChannelLabel renders a channel for prompts.
func (e *Engine) ChannelLabel(ch store.Channel, username string) string {
	if ch.Kind == "DM" {
		return fmt.Sprintf("a private direct message with @%s", username)
	}
	return "#" + ch.Name
}

// OnUserMessage decides which personas reply and streams their replies.
// It must be called in a goroutine with the session context.
func (e *Engine) OnUserMessage(ctx context.Context, ch store.Channel, user store.User, m store.Message) {
	defer func() {
		if r := recover(); r != nil {
			e.log.Error("persona engine panic", "panic", r)
		}
	}()
	responders := e.chooseResponders(ctx, ch, user, m)
	if len(responders) == 0 {
		return
	}
	mu := e.lock(ch.ID)
	mu.Lock()
	defer mu.Unlock()
	for _, pid := range responders {
		if ctx.Err() != nil {
			return
		}
		e.Reply(ctx, ch, user, pid, m.ParentID)
	}
}

func (e *Engine) chooseResponders(ctx context.Context, ch store.Channel, user store.User, m store.Message) []string {
	if ch.Kind == "DM" && ch.PersonaID != nil {
		return []string{*ch.PersonaID}
	}
	var out []string
	for _, tok := range chat.ParseMentions(m.Content) {
		if pid, ok := e.cat.ByMention(tok); ok {
			out = append(out, pid)
		}
	}
	if len(out) > 0 {
		if len(out) > maxResponders {
			out = out[:maxResponders]
		}
		return out
	}
	// thread reply to a persona message -> that persona
	if m.ParentID != nil {
		if parent, err := e.q.GetMessage(ctx, *m.ParentID); err == nil && parent.PersonaID != nil {
			return []string{*parent.PersonaID}
		}
	}
	if pid := e.classify(ctx, ch, user, m); pid != "" {
		return []string{pid}
	}
	return nil
}

// classify asks the fast model who should answer; falls back to keywords.
func (e *Engine) classify(ctx context.Context, ch store.Channel, user store.User, m store.Message) string {
	hist, _ := e.chat.History(ctx, ch.ID, 8)
	var sb strings.Builder
	for _, p := range e.cat.All() {
		fmt.Fprintf(&sb, "- %s: %s, %s\n", p.ID, p.DisplayName, p.Role)
	}
	prompt := fmt.Sprintf(`A Slack workspace has these colleagues:
%s
Channel: #%s. Recent messages:
%s
The user @%s just wrote the last message. Which ONE colleague would naturally reply? In #watercooler (casual chat) or if the message needs no reply, answer "none".
Respond with ONLY JSON: {"personaId": "sarah" | "alex" | "david" | "none"}`, sb.String(), ch.Name, e.transcript(hist, user.Username), user.Username)
	var out struct {
		PersonaID string `json:"personaId"`
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	temp := 0.0
	text, err := e.llm.Complete(cctx, llm.Request{Model: e.model, MaxTokens: 50, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: prompt}}})
	if err == nil {
		if raw, jerr := llm.ExtractJSON(text); jerr == nil && json.Unmarshal([]byte(raw), &out) == nil {
			// parsed
		} else {
			out.PersonaID = firstPersonaWord(text) // model answered in prose
		}
		pid := strings.ToLower(strings.TrimSpace(out.PersonaID))
		if pid == "none" {
			return ""
		}
		if _, ok := e.cat.Get(pid); ok {
			return pid
		}
	} else {
		e.log.Warn("responder classify failed, using heuristic", "err", err)
	}
	return e.keywordResponder(ch, m.Content)
}

// firstPersonaWord finds the first persona id or "none" in free text.
func firstPersonaWord(text string) string {
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z')
	}) {
		switch w {
		case "sarah", "alex", "david", "none":
			return w
		}
	}
	return ""
}

// keywordResponder is the offline heuristic for choosing a responder.
func (e *Engine) keywordResponder(ch store.Channel, content string) string {
	if ch.Name == "watercooler" {
		return ""
	}
	lc := strings.ToLower(content)
	best, bestScore := "", 0
	for _, p := range e.cat.All() {
		score := 0
		for _, k := range p.Keywords {
			if strings.Contains(lc, k) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = p.ID, score
		}
	}
	if best != "" {
		return best
	}
	for _, p := range e.cat.All() {
		for _, c := range p.DefaultChannels {
			if c == ch.Name {
				return p.ID
			}
		}
	}
	return "sarah"
}

func (e *Engine) speaker(m store.Message, username string) string {
	switch m.AuthorType {
	case chat.AuthorUser:
		return "@" + username
	case chat.AuthorPersona:
		if m.PersonaID != nil {
			if p, ok := e.cat.Get(*m.PersonaID); ok {
				return fmt.Sprintf("@%s (%s)", p.Handle, p.DisplayName)
			}
		}
		return "unknown"
	default:
		return "[system]"
	}
}

func (e *Engine) transcript(hist []store.Message, username string) string {
	var sb strings.Builder
	for _, h := range hist {
		if strings.TrimSpace(h.Content) == "" {
			continue
		}
		thread := ""
		if h.ParentID != nil {
			thread = " (thread reply)"
		}
		fmt.Fprintf(&sb, "%s%s: %s\n", e.speaker(h, username), thread, h.Content)
	}
	if sb.Len() == 0 {
		return "(no messages yet)\n"
	}
	return sb.String()
}

// Reply streams one persona message into the channel (or thread).
func (e *Engine) Reply(ctx context.Context, ch store.Channel, user store.User, personaID string, parentID *uuid.UUID) {
	p, ok := e.cat.Get(personaID)
	if !ok {
		return
	}
	hist, err := e.chat.History(ctx, ch.ID, historySize)
	if err != nil {
		e.log.Error("load history", "err", err)
		return
	}
	sys := e.cat.SystemPrompt(p, user.Username, e.ChannelLabel(ch, user.Username))
	userPrompt := fmt.Sprintf("Conversation so far in %s (oldest first):\n%s\nWrite your next message as %s (@%s). Reply with the message text only.",
		e.ChannelLabel(ch, user.Username), e.transcript(hist, user.Username), p.DisplayName, p.Handle)

	e.chat.Typing(ch.SessionID, ch.ID, p.ID)
	msg, err := e.chat.StartPersonaStream(ctx, ch.SessionID, ch.ID, p.ID, parentID)
	if err != nil {
		e.log.Error("start persona stream", "err", err)
		return
	}
	sctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	full, err := e.llm.Stream(sctx, llm.Request{
		Model: e.model, MaxTokens: 400,
		Messages: []llm.Message{{Role: llm.RoleSystem, Content: sys}, {Role: llm.RoleUser, Content: userPrompt}},
	}, func(d string) { e.chat.Delta(msg, d) })
	full = cleanReply(full, p)

	// Persist with a fresh context so a cancelled session still finalises the message.
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer fcancel()
	if err != nil || full == "" {
		e.log.Error("persona stream failed", "persona", p.ID, "err", err)
		if full == "" {
			full = "_(" + p.DisplayName + " seems to be offline right now — try again in a moment.)_"
		}
	}
	final, ferr := e.chat.FinishPersonaStream(fctx, msg, full)
	if ferr != nil {
		e.log.Error("finish persona stream", "err", ferr)
		return
	}
	if err == nil && ctx.Err() == nil {
		e.maybeCreateDeadline(ctx, final, user.Username)
	}
}

// cleanReply strips a leading "Name:" prefix the model sometimes adds.
func cleanReply(s string, p Persona) string {
	s = strings.TrimSpace(s)
	for _, pre := range []string{"@" + p.Handle + ":", p.Handle + ":", p.DisplayName + ":", strings.Fields(p.DisplayName)[0] + ":"} {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(pre)) {
			s = strings.TrimSpace(s[len(pre):])
		}
	}
	return s
}

// NeedsResponse reports whether a persona message addressed to the user
// requires a reply (heuristic: @mention plus question mark).
func NeedsResponse(content, username string) bool {
	return chat.Mentions(content, username) && strings.Contains(content, "?")
}

func (e *Engine) maybeCreateDeadline(ctx context.Context, m store.Message, username string) {
	if !chat.Mentions(m.Content, username) {
		return
	}
	need := strings.Contains(m.Content, "?")
	if !need {
		need = e.askIsRequest(ctx, m.Content, username)
	}
	if !need {
		return
	}
	if e.deadlines.HasPending(ctx, m.ChannelID) {
		return // one open request per channel at a time
	}
	if _, err := e.deadlines.Create(ctx, m.SessionID, m.ChannelID, m.ID, nil); err != nil {
		e.log.Error("create deadline", "err", err)
	}
}

func (e *Engine) askIsRequest(ctx context.Context, content, username string) bool {
	var out struct {
		Request bool `json:"request"`
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	temp := 0.0
	err := llm.CompleteJSON(cctx, e.llm, llm.Request{Model: e.model, MaxTokens: 30, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf(
			"Does this Slack message ask @%s a question or request something that needs a reply from them?\n\n%s\n\nRespond with ONLY JSON: {\"request\": true|false}", username, content)}}}, &out)
	return err == nil && out.Request
}

// PostStatic posts a non-LLM persona message (e.g. greeting) and creates a deadline if needed.
func (e *Engine) PostStatic(ctx context.Context, ch store.Channel, username, personaID, content string) (store.Message, error) {
	m, err := e.chat.PostPersona(ctx, ch.SessionID, ch.ID, personaID, content, nil)
	if err != nil {
		return m, err
	}
	if NeedsResponse(content, username) {
		if _, err := e.deadlines.Create(ctx, ch.SessionID, ch.ID, m.ID, nil); err != nil {
			e.log.Error("create deadline", "err", err)
		}
	}
	return m, nil
}
