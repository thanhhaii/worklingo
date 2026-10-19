// Package dto contains the JSON shapes defined in docs/CONTRACT.md and
// converters from the store models.
package dto

import (
	"encoding/json"
	"time"

	"github.com/worklingo/backend/internal/store"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"createdAt"`
}

type Persona struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	AvatarColor string `json:"avatarColor"`
}

type Session struct {
	ID        string     `json:"id"`
	Mode      string     `json:"mode"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"startedAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	EndedAt   *time.Time `json:"endedAt"`
	Warnings  int        `json:"warnings"`
}

type Channel struct {
	ID        string  `json:"id"`
	SessionID string  `json:"sessionId"`
	Kind      string  `json:"kind"`
	Name      string  `json:"name"`
	PersonaID *string `json:"personaId"`
}

type Message struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionId"`
	ChannelID  string    `json:"channelId"`
	ParentID   *string   `json:"parentId"`
	AuthorType string    `json:"authorType"`
	UserID     *string   `json:"userId"`
	PersonaID  *string   `json:"personaId"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"createdAt"`
	Streaming  *bool     `json:"streaming,omitempty"`
}

type Task struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"sessionId"`
	ChannelID   string    `json:"channelId"`
	MessageID   string    `json:"messageId"`
	PersonaID   string    `json:"personaId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Deadline struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"sessionId"`
	ChannelID  string     `json:"channelId"`
	MessageID  string     `json:"messageId"`
	TaskID     *string    `json:"taskId"`
	Status     string     `json:"status"`
	DeadlineAt time.Time  `json:"deadlineAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	ResolvedAt *time.Time `json:"resolvedAt"`
}

type Warning struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionId"`
	DeadlineID string    `json:"deadlineId"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Correction struct {
	Original    string `json:"original"`
	Corrected   string `json:"corrected"`
	Explanation string `json:"explanation"`
	Category    string `json:"category"`
}

type Evaluation struct {
	ID          string       `json:"id"`
	SessionID   string       `json:"sessionId"`
	Status      string       `json:"status"`
	Grammar     int          `json:"grammar"`
	Tone        int          `json:"tone"`
	Vocabulary  int          `json:"vocabulary"`
	Overall     int          `json:"overall"`
	Summary     string       `json:"summary"`
	Corrections []Correction `json:"corrections"`
	CreatedAt   time.Time    `json:"createdAt"`
}

type SessionDetail struct {
	Session   Session    `json:"session"`
	Channels  []Channel  `json:"channels"`
	Tasks     []Task     `json:"tasks"`
	Deadlines []Deadline `json:"deadlines"`
	Warnings  []Warning  `json:"warnings"`
}

func utc(t time.Time) time.Time { return t.UTC() }

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func FromUser(u store.User) User {
	return User{ID: u.ID.String(), Username: u.Username, Email: u.Email, CreatedAt: utc(u.CreatedAt)}
}

func FromSession(s store.Session) Session {
	return Session{
		ID: s.ID.String(), Mode: s.Mode, Status: s.Status,
		StartedAt: utc(s.StartedAt), ExpiresAt: utc(s.ExpiresAt), EndedAt: utcPtr(s.EndedAt),
		Warnings: int(s.Warnings),
	}
}

func FromChannel(c store.Channel) Channel {
	return Channel{ID: c.ID.String(), SessionID: c.SessionID.String(), Kind: c.Kind, Name: c.Name, PersonaID: c.PersonaID}
}

func FromMessage(m store.Message) Message {
	out := Message{
		ID: m.ID.String(), SessionID: m.SessionID.String(), ChannelID: m.ChannelID.String(),
		AuthorType: m.AuthorType, PersonaID: m.PersonaID, Content: m.Content, CreatedAt: utc(m.CreatedAt),
	}
	if m.ParentID != nil {
		s := m.ParentID.String()
		out.ParentID = &s
	}
	if m.UserID != nil {
		s := m.UserID.String()
		out.UserID = &s
	}
	return out
}

// FromMessageStreaming sets the streaming flag (only used in WS events).
func FromMessageStreaming(m store.Message, streaming bool) Message {
	out := FromMessage(m)
	out.Streaming = &streaming
	return out
}

func FromTask(t store.Task) Task {
	return Task{
		ID: t.ID.String(), SessionID: t.SessionID.String(), ChannelID: t.ChannelID.String(),
		MessageID: t.MessageID.String(), PersonaID: t.PersonaID, Title: t.Title, Description: t.Description,
		Priority: t.Priority, Status: t.Status, CreatedAt: utc(t.CreatedAt),
	}
}

func FromDeadline(d store.Deadline) Deadline {
	out := Deadline{
		ID: d.ID.String(), SessionID: d.SessionID.String(), ChannelID: d.ChannelID.String(),
		MessageID: d.MessageID.String(), Status: d.Status, DeadlineAt: utc(d.DeadlineAt),
		CreatedAt: utc(d.CreatedAt), ResolvedAt: utcPtr(d.ResolvedAt),
	}
	if d.TaskID != nil {
		s := d.TaskID.String()
		out.TaskID = &s
	}
	return out
}

func FromWarning(w store.Warning) Warning {
	return Warning{ID: w.ID.String(), SessionID: w.SessionID.String(), DeadlineID: w.DeadlineID.String(), Reason: w.Reason, CreatedAt: utc(w.CreatedAt)}
}

func FromEvaluation(e store.Evaluation) Evaluation {
	out := Evaluation{
		ID: e.ID.String(), SessionID: e.SessionID.String(), Status: e.Status,
		Grammar: int(e.Grammar), Tone: int(e.Tone), Vocabulary: int(e.Vocabulary), Overall: int(e.Overall),
		Summary: e.Summary, CreatedAt: utc(e.CreatedAt), Corrections: []Correction{},
	}
	if len(e.Corrections) > 0 {
		_ = json.Unmarshal(e.Corrections, &out.Corrections)
		if out.Corrections == nil {
			out.Corrections = []Correction{}
		}
	}
	return out
}

func Map[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}
