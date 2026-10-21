// Package dto contains the JSON shapes defined in docs/CONTRACT.md and
// converters from the store models.
package dto

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

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

	WorkContext *WorkContext `json:"workContext"`
}

// WorkContext is the fictional project situation of a session (stored as JSONB, exposed as-is).
type WorkContext struct {
	Project        string   `json:"project"`
	ProjectSummary string   `json:"projectSummary"`
	Feature        string   `json:"feature"`
	Progress       string   `json:"progress"`
	Blocker        string   `json:"blocker"`
	Notes          []string `json:"notes"`
}

// Describe renders the work context as plain text for LLM prompts.
func (w WorkContext) Describe() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- Project: %s (%s)\n- Feature the learner is building: %s\n- Progress: %s\n- Blocker: %s\n",
		w.Project, w.ProjectSummary, w.Feature, w.Progress, w.Blocker)
	for _, n := range w.Notes {
		fmt.Fprintf(&sb, "- %s\n", n)
	}
	return strings.TrimSpace(sb.String())
}

// ParseWorkContext decodes the sessions.work_context column (nil if absent or invalid).
func ParseWorkContext(raw []byte) *WorkContext {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var w WorkContext
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil
	}
	if w.Notes == nil {
		w.Notes = []string{}
	}
	return &w
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
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	ChannelID string    `json:"channelId"`
	MessageID string    `json:"messageId"`
	PersonaID string    `json:"personaId"`
	Title     string    `json:"title"`
	Priority  string    `json:"priority"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`

	TalkingPoints []TalkingPoint `json:"talkingPoints"`
}

type TalkingPoint struct {
	ID       string `json:"id"`
	Position int    `json:"position"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
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

type MessageIssue struct {
	Original    string `json:"original"`
	Corrected   string `json:"corrected"`
	Explanation string `json:"explanation"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
}

type MessageReview struct {
	MessageID    string         `json:"messageId"`
	ChannelLabel string         `json:"channelLabel"`
	Content      string         `json:"content"`
	CreatedAt    time.Time      `json:"createdAt"`
	Issues       []MessageIssue `json:"issues"`
}

type TalkingPointReview struct {
	TalkingPointID string `json:"talkingPointId"`
	Content        string `json:"content"`
	Covered        bool   `json:"covered"`
	LearnerQuote   string `json:"learnerQuote"`
	Reason         string `json:"reason"`
	Sample         string `json:"sample"`
}

type TaskReview struct {
	TaskID        string               `json:"taskId"`
	Title         string               `json:"title"`
	PersonaID     string               `json:"personaId"`
	Status        string               `json:"status"`
	TalkingPoints []TalkingPointReview `json:"talkingPoints"`
}

type Evaluation struct {
	ID             string          `json:"id"`
	SessionID      string          `json:"sessionId"`
	Status         string          `json:"status"`
	Grammar        int             `json:"grammar"`
	Tone           int             `json:"tone"`
	Vocabulary     int             `json:"vocabulary"`
	Overall        int             `json:"overall"`
	Summary        string          `json:"summary"`
	CreatedAt      time.Time       `json:"createdAt"`
	MessageReviews []MessageReview `json:"messageReviews"`
	TaskReviews    []TaskReview    `json:"taskReviews"`
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
		Warnings: int(s.Warnings), WorkContext: ParseWorkContext(s.WorkContext),
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

// FromTask converts a task with its talking points (any order; sorted by position).
func FromTask(t store.Task, points []store.TalkingPoint) Task {
	tps := make([]TalkingPoint, 0, len(points))
	for _, p := range points {
		tps = append(tps, TalkingPoint{ID: p.ID.String(), Position: int(p.Position), Content: p.Content, Status: p.Status, Reason: p.Reason})
	}
	sort.SliceStable(tps, func(i, j int) bool { return tps[i].Position < tps[j].Position })
	return Task{
		ID: t.ID.String(), SessionID: t.SessionID.String(), ChannelID: t.ChannelID.String(),
		MessageID: t.MessageID.String(), PersonaID: t.PersonaID, Title: t.Title,
		Priority: t.Priority, Status: t.Status, CreatedAt: utc(t.CreatedAt), TalkingPoints: tps,
	}
}

// LoadTask converts a task, loading its talking points from the database.
func LoadTask(ctx context.Context, q *store.Queries, t store.Task) Task {
	pts, _ := q.ListTalkingPointsForTask(ctx, t.ID)
	return FromTask(t, pts)
}

// FromTasks converts a session's tasks, grouping the session's talking points.
func FromTasks(ts []store.Task, points []store.TalkingPoint) []Task {
	by := map[uuid.UUID][]store.TalkingPoint{}
	for _, p := range points {
		by[p.TaskID] = append(by[p.TaskID], p)
	}
	out := make([]Task, 0, len(ts))
	for _, t := range ts {
		out = append(out, FromTask(t, by[t.ID]))
	}
	return out
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
		Summary: e.Summary, CreatedAt: utc(e.CreatedAt), MessageReviews: []MessageReview{}, TaskReviews: []TaskReview{},
	}
	if len(e.MessageReviews) > 0 {
		_ = json.Unmarshal(e.MessageReviews, &out.MessageReviews)
	}
	if len(e.TaskReviews) > 0 {
		_ = json.Unmarshal(e.TaskReviews, &out.TaskReviews)
	}
	if out.MessageReviews == nil {
		out.MessageReviews = []MessageReview{}
	}
	for i := range out.MessageReviews {
		if out.MessageReviews[i].Issues == nil {
			out.MessageReviews[i].Issues = []MessageIssue{}
		}
	}
	if out.TaskReviews == nil {
		out.TaskReviews = []TaskReview{}
	}
	for i := range out.TaskReviews {
		if out.TaskReviews[i].TalkingPoints == nil {
			out.TaskReviews[i].TalkingPoints = []TalkingPointReview{}
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
