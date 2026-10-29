package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/worklingo/backend/db/migrations"
	"github.com/worklingo/backend/internal/auth"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/evaluation"
	"github.com/worklingo/backend/internal/http/handlers"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

// These tests refuse every database except the disposable controller-provided DB.
func evaluationDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL is required for scratch database integration tests")
	}
	if raw != "postgres://worklingo:worklingo@localhost:5433/worklingo_codex_20261029?sslmode=disable" {
		t.Fatal("refusing non-scratch database")
	}
	pool, err := pgxpool.New(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	return pool
}

type evaluationProvider struct {
	mu        sync.Mutex
	calls     []string
	failScore bool
	block     chan struct{}
	entered   chan struct{}
}

func (p *evaluationProvider) Stream(context.Context, llm.Request, func(string)) (string, error) {
	panic("unexpected stream")
}
func (p *evaluationProvider) Complete(ctx context.Context, r llm.Request) (string, error) {
	p.mu.Lock()
	kind := "messages"
	if len(r.Messages) > 1 {
		kind = "score"
	} else if strings.Contains(r.Messages[0].Content, "Task t1:") {
		kind = "tasks"
	}
	p.calls = append(p.calls, kind)
	n := len(p.calls)
	fail := p.failScore
	block, entered := p.block, p.entered
	p.mu.Unlock()
	if block != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	switch kind {
	case "score":
		if fail {
			return "", errors.New("offline")
		}
		return `{"grammar":75,"tone":80,"vocabulary":85,"summary":"Specific feedback."}`, nil
	case "tasks":
		return `{"tasks":[{"id":"t1","points":[{"index":0,"learnerQuote":"","reason":"Needs detail","sample":"Here is the update."}]}]}`, nil
	default:
		if n > 3 && strings.Contains(r.Messages[0].Content, "m1 [#general]: retained learner message") {
			return "", errors.New("successful message was billed again")
		}
		return `{"messages":[{"id":"m1","issues":[]}]}`, nil
	}
}
func (p *evaluationProvider) count(kind string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, k := range p.calls {
		if k == kind {
			n++
		}
	}
	return n
}

type evalFixture struct {
	pool         *pgxpool.Pool
	q            *store.Queries
	svc          *evaluation.Service
	provider     *evaluationProvider
	router       http.Handler
	session      store.Session
	owner, other string
}

func newEvalFixture(t *testing.T, active bool) *evalFixture {
	t.Helper()
	pool := evaluationDB(t)
	q := store.New(pool)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	uid := uuid.New()
	u, err := q.CreateUser(ctx, store.CreateUserParams{Username: "test_" + strings.ReplaceAll(uid.String(), "-", ""), Email: uid.String() + "@test.local", PasswordHash: "unused"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", u.ID) })
	sess, err := q.CreateSession(ctx, store.CreateSessionParams{UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := q.CreateChannel(ctx, store.CreateChannelParams{SessionID: sess.ID, Kind: "CHANNEL", Name: "general"})
	if err != nil {
		t.Fatal(err)
	}
	req, err := q.CreateMessage(ctx, store.CreateMessageParams{SessionID: sess.ID, ChannelID: ch.ID, AuthorType: "PERSONA", Content: "Send an update"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := q.CreateTask(ctx, store.CreateTaskParams{SessionID: sess.ID, ChannelID: ch.ID, MessageID: req.ID, PersonaID: "alex", Title: "Update", Priority: "NORMAL"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.CreateTalkingPoint(ctx, store.CreateTalkingPointParams{TaskID: task.ID, Content: "Share progress"}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"retained learner message", "missing learner message"} {
		if _, err = q.CreateMessage(ctx, store.CreateMessageParams{SessionID: sess.ID, ChannelID: ch.ID, AuthorType: "USER", UserID: &u.ID, Content: text}); err != nil {
			t.Fatal(err)
		}
	}
	if !active {
		sess, err = q.CompleteSession(ctx, sess.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	cat, err := persona.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	hub := ws.NewHub(log, nil)
	p := &evaluationProvider{}
	svc := evaluation.New(ctx, q, cat, hub, p, "fixture", log)
	jwt := auth.NewJWT("test-secret", time.Hour)
	owner, _ := jwt.Issue(u.ID)
	other, _ := jwt.Issue(uuid.New())
	h := &handlers.Handlers{Q: q, Log: log, Evaluations: svc}
	router := NewRouter(Deps{H: h, Hub: hub, JWT: jwt, Log: log})
	return &evalFixture{pool, q, svc, p, router, sess, owner, other}
}
func (f *evalFixture) post(token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/sessions/"+f.session.ID.String()+"/evaluation/retry", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	return w
}
func (f *evalFixture) wait(t *testing.T, status string) store.Evaluation {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		e, err := f.q.GetEvaluation(context.Background(), f.session.ID)
		if err == nil && e.Status == status {
			return e
		}
		time.Sleep(5 * time.Millisecond)
	}
	e, err := f.q.GetEvaluation(context.Background(), f.session.ID)
	t.Fatalf("waiting for %s, got %+v, %v", status, e, err)
	return e
}

func TestEvaluationRetryPartialRetainsSuccessAndClaimsOnce(t *testing.T) {
	f := newEvalFixture(t, false)
	f.svc.Start(f.session.ID)
	before := f.wait(t, "PARTIAL")
	if before.ScoreStatus != "AVAILABLE" || before.TaskReviewStatus != "AVAILABLE" {
		t.Fatalf("success lost: %+v", before)
	}
	if w := f.post(""); w.Code != 401 {
		t.Fatalf("unauthorized: %d", w.Code)
	}
	if w := f.post(f.other); w.Code != 404 {
		t.Fatalf("ownership: %d", w.Code)
	}
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	f.provider.mu.Lock()
	f.provider.block = release
	f.provider.entered = entered
	f.provider.mu.Unlock()
	defer close(release)
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- f.post(f.owner).Code }()
	}
	wg.Wait()
	close(codes)
	accepted, conflicts := 0, 0
	for code := range codes {
		switch code {
		case 202:
			accepted++
		case 409:
			conflicts++
		default:
			t.Errorf("unexpected retry status %d", code)
		}
	}
	if accepted != 1 || conflicts != 11 {
		t.Fatalf("got %d accepted and %d conflicts", accepted, conflicts)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("retry did not dispatch")
	}
	release <- struct{}{}
	after := f.wait(t, "DONE")
	if after.Grammar != before.Grammar || after.Summary != before.Summary || string(after.TaskReviews) != string(before.TaskReviews) {
		t.Fatal("successful scores/task component changed")
	}
	old, new := dto.FromEvaluation(before), dto.FromEvaluation(after)
	a, _ := json.Marshal(old.MessageReviews[0])
	b, _ := json.Marshal(new.MessageReviews[0])
	if string(a) != string(b) {
		t.Fatal("successful message changed")
	}
	if f.provider.count("score") != 1 || f.provider.count("tasks") != 1 || f.provider.count("messages") != 2 {
		t.Fatalf("successful work was repeated: %+v", f.provider.calls)
	}
	if w := f.post(f.owner); w.Code != 409 {
		t.Fatalf("completed retry: %d", w.Code)
	}
}
func TestEvaluationRetryRejectsActiveSession(t *testing.T) {
	f := newEvalFixture(t, true)
	if _, err := f.pool.Exec(context.Background(), "INSERT INTO evaluations (session_id,status) VALUES ($1,'FAILED')", f.session.ID); err != nil {
		t.Fatal(err)
	}
	if w := f.post(f.owner); w.Code != 409 {
		t.Fatalf("active retry got %d", w.Code)
	}
	if len(f.provider.calls) != 0 {
		t.Fatal("active session dispatched")
	}
}
func TestEvaluationRetryFailedScoresKeepsReviews(t *testing.T) {
	f := newEvalFixture(t, false)
	f.provider.failScore = true
	f.svc.Start(f.session.ID)
	before := f.wait(t, "FAILED")
	if before.TaskReviewStatus != "AVAILABLE" {
		t.Fatal("task review lost on scoring failure")
	}
	f.provider.mu.Lock()
	f.provider.failScore = false
	f.provider.mu.Unlock()
	if w := f.post(f.owner); w.Code != 202 {
		t.Fatalf("failed retry got %d: %s", w.Code, w.Body.String())
	}
	after := f.wait(t, "DONE")
	if string(before.TaskReviews) != string(after.TaskReviews) || f.provider.count("tasks") != 1 {
		t.Fatal("successful task review was regenerated")
	}
}

func TestEvaluationStartAndRecoveryDoNotDoubleDispatch(t *testing.T) {
	f := newEvalFixture(t, false)
	release := make(chan struct{})
	entered := make(chan struct{}, 10)
	f.provider.mu.Lock()
	f.provider.block = release
	f.provider.entered = entered
	f.provider.mu.Unlock()
	f.svc.Start(f.session.ID)
	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("initial evaluation did not start")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.svc.Start(f.session.ID); f.svc.Recover(context.Background()) }()
	}
	wg.Wait()
	close(release)
	f.wait(t, "PARTIAL")
	if f.provider.count("score") != 1 || f.provider.count("tasks") != 1 || f.provider.count("messages") != 1 {
		t.Fatalf("double dispatch: %+v", f.provider.calls)
	}
	f.svc.Start(f.session.ID)
	e, err := f.q.GetEvaluation(context.Background(), f.session.ID)
	if err != nil || e.Status != "PARTIAL" {
		t.Fatalf("Start reset a completed row: %+v %v", e, err)
	}
}

func TestEvaluationRecoveryRetainsPersistedComponents(t *testing.T) {
	f := newEvalFixture(t, false)
	ctx := context.Background()
	if _, err := f.q.UpsertPendingEvaluation(ctx, f.session.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.q.SaveEvaluationScores(ctx, store.SaveEvaluationScoresParams{SessionID: f.session.ID, Grammar: 93, Tone: 92, Vocabulary: 91, Overall: 92, Summary: "Persisted feedback."}); err != nil {
		t.Fatal(err)
	}
	msgs, err := f.q.ListSessionMessages(ctx, f.session.ID)
	if err != nil {
		t.Fatal(err)
	}
	reviews := []dto.MessageReview{}
	for _, m := range msgs {
		if m.AuthorType == "USER" {
			reviews = append(reviews, dto.MessageReview{MessageID: m.ID.String(), Content: m.Content, ReviewStatus: "AVAILABLE", Issues: []dto.MessageIssue{}})
		}
	}
	raw, _ := json.Marshal(reviews)
	if err := f.q.SaveEvaluationMessageReviews(ctx, store.SaveEvaluationMessageReviewsParams{SessionID: f.session.ID, MessageReviews: raw, MessageReviewStatus: "AVAILABLE"}); err != nil {
		t.Fatal(err)
	}
	f.svc.Recover(ctx)
	e := f.wait(t, "DONE")
	if e.Grammar != 93 || e.Summary != "Persisted feedback." || f.provider.count("score") != 0 || f.provider.count("messages") != 0 || f.provider.count("tasks") != 1 {
		t.Fatalf("recovery repeated persisted work: %+v, calls=%+v", e, f.provider.calls)
	}
}

func TestEvaluationNoLearnerMessagesNeedsNoPaidCalls(t *testing.T) {
	f := newEvalFixture(t, false)
	if _, err := f.pool.Exec(context.Background(), "DELETE FROM messages WHERE session_id=$1 AND author_type='USER'", f.session.ID); err != nil {
		t.Fatal(err)
	}
	f.svc.Start(f.session.ID)
	e := f.wait(t, "DONE")
	if e.Overall != 0 || e.ScoreStatus != "AVAILABLE" || f.provider.count("score")+f.provider.count("messages")+f.provider.count("tasks") != 0 {
		t.Fatalf("empty session called provider: %+v", e)
	}
}

func TestEvaluationMigrationPreservesScoresAndMarksLegacyCoverageUnknown(t *testing.T) {
	pool := evaluationDB(t)
	ctx := context.Background()
	schema := "eval_migration_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	// The random schema is created only inside the allowlisted scratch database.
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg := pool.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	isolated, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { isolated.Close(); _, _ = pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	db := stdlib.OpenDBFromPool(isolated)
	defer db.Close()
	if err = goose.UpTo(db, ".", 2); err != nil {
		t.Fatal(err)
	}
	user, session, ev := uuid.New(), uuid.New(), uuid.New()
	if _, err = isolated.Exec(ctx, "INSERT INTO users(id,username,email,password_hash) VALUES($1,'legacy','legacy@test.local','unused')", user); err != nil {
		t.Fatal(err)
	}
	if _, err = isolated.Exec(ctx, "INSERT INTO sessions(id,user_id,expires_at,status) VALUES($1,$2,now(),'COMPLETED')", session, user); err != nil {
		t.Fatal(err)
	}
	if _, err = isolated.Exec(ctx, `INSERT INTO evaluations(id,session_id,status,grammar,summary,message_reviews) VALUES($1,$2,'DONE',89,'Keep legacy score','[{"messageId":"legacy","issues":[]}]')`, ev, session); err != nil {
		t.Fatal(err)
	}
	if err = goose.Up(db, "."); err != nil {
		t.Fatal(err)
	}
	row, err := store.New(isolated).GetEvaluation(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	converted := dto.FromEvaluation(row)
	if row.Grammar != 89 || row.Summary != "Keep legacy score" || converted.Status != "PARTIAL" || converted.ScoreStatus != "AVAILABLE" || converted.MessageReviewStatus != "UNAVAILABLE" || converted.TaskReviewStatus != "UNAVAILABLE" || converted.MessageReviews[0].ReviewStatus != "UNAVAILABLE" {
		t.Fatalf("legacy integrity lost: %+v", converted)
	}
	if err = goose.DownTo(db, ".", 2); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = isolated.QueryRow(ctx, "SELECT status FROM evaluations WHERE id=$1", ev).Scan(&status); err != nil || status != "DONE" {
		t.Fatalf("rollback: %s %v", status, err)
	}
}
