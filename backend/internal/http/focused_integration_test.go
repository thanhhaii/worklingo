package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/auth"
	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/evaluation"
	"github.com/worklingo/backend/internal/http/handlers"
	"github.com/worklingo/backend/internal/lifecycle"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/scenario"
	"github.com/worklingo/backend/internal/session"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/task"
	"github.com/worklingo/backend/internal/validator"
	"github.com/worklingo/backend/internal/workctx"
	"github.com/worklingo/backend/internal/ws"
)

type focusProvider struct {
	mu               sync.Mutex
	complete, stream []llm.Request
	partial          bool
}

func (p *focusProvider) Complete(_ context.Context, r llm.Request) (string, error) {
	p.mu.Lock()
	p.complete = append(p.complete, r)
	partial := p.partial
	p.mu.Unlock()
	s := r.Messages[0].Content
	if strings.Contains(s, "Pending talking points") {
		if partial {
			return `{"points":[{"index":0,"covered":true,"reason":"Đã nêu"},{"index":1,"covered":false,"reason":"Thiếu blocker"}]}`, nil
		}
		return `{"points":[{"index":0,"covered":true,"reason":"Đã nêu"},{"index":1,"covered":true,"reason":"Đã nêu"}]}`, nil
	}
	if strings.Contains(s, "Business English coach") && len(r.Messages) > 1 {
		return `{"grammar":80,"tone":80,"vocabulary":80,"summary":"Goal feedback"}`, nil
	}
	if strings.Contains(s, "Tasks:") {
		return `{"tasks":[{"id":"t1","points":[{"index":0,"reason":"OK","sample":"Progress is 70%."},{"index":1,"reason":"OK","sample":"Waiting on security rules."}]}]}`, nil
	}
	if strings.Contains(s, "Messages:") {
		return `{"messages":[{"id":"m1","issues":[]}]}`, nil
	}
	return `{}`, nil
}
func (p *focusProvider) Stream(_ context.Context, r llm.Request, f func(string)) (string, error) {
	p.mu.Lock()
	p.stream = append(p.stream, r)
	p.mu.Unlock()
	f("Thanks. ")
	return "Thanks. @learner please keep me updated.", nil
}
func TestFocusedCreateReplyAndEvaluation(t *testing.T) {
	pool := evaluationDB(t)
	q := store.New(pool)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := ws.NewHub(log, nil)
	cat, _ := persona.LoadCatalog()
	p := &focusProvider{}
	reg := lifecycle.NewRegistry(ctx)
	c := chat.NewService(q, hub)
	d := deadline.NewService(q, c, hub, validator.New(p, "fixture", log), reg, time.Minute, log)
	e := persona.NewEngine(cat, q, c, d, p, "fixture", log)
	sch := scenario.New(scenario.Config{FirstDelay: time.Hour}, pool, q, cat, c, d, hub, p, "fixture", log)
	ev := evaluation.New(ctx, q, cat, hub, p, "fixture", log)
	manager := session.NewManager(pool, q, reg, e, sch, d, ev, workctx.NewGenerator(p, "fixture", log), hub, 2*time.Second, log)
	defer manager.Shutdown()
	tasks := task.NewService(q, e, d, hub, reg, p, "fixture", log)
	uid := uuid.New().String()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Username: "learner" + uid[:8], Email: uid + "@test.local", PasswordHash: "unused"})
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM users WHERE id=$1", u.ID)
	jwt := auth.NewJWT("test", time.Hour)
	token, _ := jwt.Issue(u.ID)
	h := &handlers.Handlers{Q: q, Sessions: manager, Engine: e, Chat: c, Tasks: tasks, Registry: reg, Log: log}
	router := NewRouter(Deps{H: h, Hub: hub, JWT: jwt, Log: log})
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/sessions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{"durationMinutes":null}`, `{"learningGoal":null}`, `{"durationMinutes":5}`, `{"learningGoal":"GRAMMAR"}`, `{"durationMinutes":"10"}`} {
		if w := post(body); w.Code != 400 {
			t.Fatalf("%s: %d", body, w.Code)
		}
	}
	// Legacy requests keep configured seconds and generate context/tasks/deadlines.
	legacySch := scenario.New(scenario.Config{FirstDelay: 10 * time.Millisecond, MinInterval: time.Hour, MaxInterval: time.Hour}, pool, q, cat, c, d, hub, p, "fixture", log)
	legacyManager := session.NewManager(pool, q, reg, e, legacySch, d, ev, workctx.NewGenerator(p, "fixture", log), hub, 2*time.Second, log)
	defer legacyManager.Shutdown()
	h.Sessions = legacyManager
	for _, body := range []string{"", `{}`, `{"durationMinutes":10}`} {
		res := post(body)
		if res.Code != 201 {
			t.Fatal("legacy create", res.Code, res.Body.String())
		}
		var s dto.Session
		_ = json.Unmarshal(res.Body.Bytes(), &s)
		if s.LearningGoal != nil {
			t.Fatal("legacy became focused")
		}
		want := 2 * time.Second
		if body == `{"durationMinutes":10}` {
			want = 10 * time.Minute
		}
		if delta := want - s.ExpiresAt.Sub(s.StartedAt); delta < 0 || delta > time.Second {
			t.Fatalf("configured duration changed: %v", s.ExpiresAt.Sub(s.StartedAt))
		}
		until := time.Now().Add(time.Second)
		for time.Now().Before(until) {
			rows, _ := q.ListDeadlines(ctx, uuid.MustParse(s.ID))
			if len(rows) > 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		rows, _ := q.ListDeadlines(ctx, uuid.MustParse(s.ID))
		if len(rows) == 0 {
			t.Fatal("legacy deadlines disabled")
		}
		id := uuid.MustParse(s.ID)
		reg.Stop(id)
		d.StopSession(id)
		_, _ = q.CompleteSession(ctx, id)
	}
	h.Sessions = manager
	p.mu.Lock()
	p.complete = nil
	p.stream = nil
	p.mu.Unlock()
	// Goal without explicit duration keeps the actual two-second smoke configuration.
	short := post(`{"learningGoal":"STATUS_UPDATE"}`)
	if short.Code != 201 {
		t.Fatal(short.Code)
	}
	var smoke dto.Session
	_ = json.Unmarshal(short.Body.Bytes(), &smoke)
	shortID := uuid.MustParse(smoke.ID)
	if smoke.DurationMinutes != nil || smoke.LearningGoal == nil || smoke.ExpiresAt.Sub(smoke.StartedAt) > 2*time.Second {
		t.Fatal("short default not preserved")
	}
	untilShort := time.Now().Add(1850 * time.Millisecond)
	for time.Now().Before(untilShort) {
		rows, _ := q.ListTasks(ctx, shortID)
		if len(rows) == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	rows, _ := q.ListTasks(ctx, shortID)
	if len(rows) != 2 {
		t.Fatal("short focused scheduling", len(rows))
	}
	reg.Stop(shortID)
	_, _ = q.CompleteSession(ctx, shortID)
	p.mu.Lock()
	calls := len(p.complete) + len(p.stream)
	p.mu.Unlock()
	if calls != 0 {
		t.Fatal("short focused called model", calls)
	}
	w := post(`{"learningGoal":"STATUS_UPDATE","durationMinutes":10}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		ID              string
		LearningGoal    *string
		DurationMinutes *int32
	}
	_ = json.Unmarshal(w.Body.Bytes(), &response)
	id := uuid.MustParse(response.ID)
	if response.LearningGoal == nil || response.DurationMinutes == nil {
		t.Fatal("missing metadata")
	}
	var ts []store.Task
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		ts, _ = q.ListTasks(ctx, id)
		if len(ts) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(ts) != 1 {
		t.Fatal("first focused task missing")
	}
	p.mu.Lock()
	n := len(p.complete) + len(p.stream)
	p.mu.Unlock()
	if n != 0 {
		t.Fatal("focused setup called provider", n)
	}
	ch, _ := q.GetChannel(ctx, ts[0].ChannelID)
	m, _ := c.PostUser(ctx, ch, u.ID, "@alex @sarah User management is 70% complete. We are waiting on security rules.", &ts[0].MessageID)
	tasks.OnUserMessage(ch, u, m)
	until = time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		p.mu.Lock()
		n = len(p.stream)
		p.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	p.mu.Lock()
	if len(p.complete) != 1 || len(p.stream) != 1 {
		t.Fatalf("focused turn calls complete=%d stream=%d", len(p.complete), len(p.stream))
	}
	prompt := p.stream[0].Messages[0].Content
	if !strings.Contains(prompt, "STATUS_UPDATE") || !strings.Contains(prompt, "security rules") {
		t.Fatal("goal/facts absent")
	}
	if !strings.Contains(p.stream[0].Messages[1].Content, "Sarah") {
		t.Fatal("thread parent should win over mentions")
	}
	p.mu.Unlock()
	ds, _ := q.ListDeadlines(ctx, id)
	if len(ds) != 0 {
		t.Fatal("focused deadline created")
	}
	cur, _ := q.GetTask(ctx, ts[0].ID)
	if cur.Status != "DONE" {
		t.Fatal("not completed", cur.Status)
	}
	// Focused messages with no open candidate still take one deterministic responder.
	m, _ = c.PostUser(ctx, ch, u.ID, "@alex @sarah Thanks for the update", nil)
	e.OnUserMessage(ctx, ch, u, m)
	p.mu.Lock()
	if len(p.complete) != 1 || len(p.stream) != 2 {
		t.Fatal("selection/request classifier used")
	}
	if !strings.Contains(p.stream[1].Messages[1].Content, "Alex") {
		t.Fatal("first mention must win")
	}
	p.mu.Unlock()
	if _, err = d.Create(ctx, id, ch.ID, m.ID, nil); !errors.Is(err, deadline.ErrFocusedPolicy) {
		t.Fatal("defensive deadline policy", err)
	}
	// Preserve incomplete-task acknowledgement and one follow-up, with no regular extra reply.
	_, err = pool.Exec(ctx, "UPDATE tasks SET status='OPEN' WHERE id=$1", ts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, "UPDATE talking_points SET status='PENDING' WHERE task_id=$1", ts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.partial = true
	p.mu.Unlock()
	m, _ = c.PostUser(ctx, ch, u.ID, "User management is 70% complete.", &ts[0].MessageID)
	tasks.OnUserMessage(ch, u, m)
	until = time.Now().Add(time.Second)
	for time.Now().Before(until) {
		cur, _ = q.GetTask(ctx, ts[0].ID)
		if cur.FollowUpSent {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	cur, _ = q.GetTask(ctx, ts[0].ID)
	if cur.Status != "ACKNOWLEDGED" || !cur.FollowUpSent {
		t.Fatalf("focused follow-up %+v", cur)
	}
	p.mu.Lock()
	if len(p.complete) != 2 || len(p.stream) != 3 {
		t.Fatalf("extra focused follow-up calls %d/%d", len(p.complete), len(p.stream))
	}
	p.mu.Unlock()
	ds, _ = q.ListDeadlines(ctx, id)
	if len(ds) != 0 {
		t.Fatal("follow-up created deadline")
	}
	if _, err = manager.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	until = time.Now().Add(time.Second)
	for time.Now().Before(until) {
		row, err := q.GetEvaluation(ctx, id)
		if err == nil && row.Status != "PENDING" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.complete) != 5 {
		t.Fatalf("evaluation calls: %d", len(p.complete))
	}
	for _, r := range p.complete[2:] {
		all := ""
		for _, m := range r.Messages {
			all += m.Content
		}
		if !strings.Contains(all, "STATUS_UPDATE") || !strings.Contains(all, "security rules") || !strings.Contains(all, "No response deadlines") {
			t.Fatal("evaluation omitted goal/facts/policy")
		}
	}
}
