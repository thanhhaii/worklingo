package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/http/handlers"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/practice"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/ws"
)

type practiceProvider struct {
	mu        sync.Mutex
	calls     int
	fail      bool
	malformed bool
	block     chan struct{}
	entered   chan struct{}
}

func (*practiceProvider) Stream(context.Context, llm.Request, func(string)) (string, error) {
	panic("stream")
}
func (p *practiceProvider) Complete(ctx context.Context, r llm.Request) (string, error) {
	p.mu.Lock()
	p.calls++
	fail, bad, block, entered := p.fail, p.malformed, p.block, p.entered
	p.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if fail {
		return "", errors.New("429 secret")
	}
	if bad {
		return `{"results":[]}`, nil
	}
	raw := strings.SplitN(r.Messages[0].Content, "Data: ", 2)[1]
	var in struct {
		Exercises []struct{ ExerciseID, Answer string }
	}
	_ = json.Unmarshal([]byte(raw), &in)
	results := []map[string]string{}
	for _, e := range in.Exercises {
		verdict := "ACCEPTED"
		if e.Answer == "wrong" {
			verdict = "NEEDS_WORK"
		}
		results = append(results, map[string]string{"exerciseId": e.ExerciseID, "verdict": verdict, "feedback": "Câu viết giữ đúng ý nghĩa."})
	}
	b, _ := json.Marshal(map[string]any{"results": results})
	return string(b), nil
}
func TestPracticeOwnershipMaterializationValidationAndReplay(t *testing.T) {
	f := newEvalFixture(t, false)
	p := &practiceProvider{}
	svc := practice.New(f.q, p, "eval-fixture")
	// Reuse the authenticated fixture router with the new service.
	f.router = practiceRouter(f, svc)
	ctx := context.Background()
	msgs, _ := f.q.ListSessionMessages(ctx, f.session.ID)
	reviews := []dto.MessageReview{}
	for _, m := range msgs {
		if m.AuthorType == "USER" {
			reviews = append(reviews, dto.MessageReview{MessageID: m.ID.String(), Content: m.Content, ReviewStatus: "AVAILABLE", Issues: []dto.MessageIssue{{Original: m.Content, Corrected: "I have completed the update.", Explanation: "Sửa câu đầy đủ.", Category: "GRAMMAR"}}})
		}
	}
	_, _ = f.q.UpsertPendingEvaluation(ctx, f.session.ID)
	raw, _ := json.Marshal(reviews)
	if err := f.q.SaveEvaluationMessageReviews(ctx, store.SaveEvaluationMessageReviewsParams{SessionID: f.session.ID, MessageReviews: raw, MessageReviewStatus: "AVAILABLE"}); err != nil {
		t.Fatal(err)
	}
	req := func(method, path, token, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/sessions/"+f.session.ID.String()+path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, r)
		return w
	}
	if w := req("POST", "/practice/attempts", f.other, `{}`); w.Code != 404 {
		t.Fatal("submission ownership", w.Code)
	}
	if w := req("GET", "/practice", f.other, ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE evaluations SET status='DONE' WHERE session_id=$1", f.session.ID); err != nil {
		t.Fatal(err)
	}
	var queue practice.Queue
	w := req("GET", "/practice", f.owner, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &queue)
	if len(queue.Items) != 2 {
		t.Fatal(queue)
	}
	id := queue.Items[0].ID
	var again practice.Queue
	_ = json.Unmarshal(req("GET", "/practice", f.owner, "").Body.Bytes(), &again)
	if again.Items[0].ID != id {
		t.Fatal("unstable ids")
	}
	// Existing report snapshots retain their initial rubric after later review changes.
	reviews[0].Issues[0].Corrected = "A changed sample."
	changedReview, _ := json.Marshal(reviews)
	if _, err := f.pool.Exec(ctx, "UPDATE evaluations SET message_reviews=$2 WHERE session_id=$1", f.session.ID, changedReview); err != nil {
		t.Fatal(err)
	}
	immutable, err := svc.Queue(ctx, f.session.ID, f.session.UserID)
	if err != nil || immutable.Items[0].Correction != "I have completed the update." {
		t.Fatal("snapshot mutated", immutable, err)
	}
	// A valid item belonging to a different session must still be rejected.
	otherSession, err := f.q.CreateSession(ctx, store.CreateSessionParams{UserID: f.session.UserID, ExpiresAt: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	otherChannel, err := f.q.CreateChannel(ctx, store.CreateChannelParams{SessionID: otherSession.ID, Kind: "CHANNEL", Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	otherMessage, err := f.q.CreateMessage(ctx, store.CreateMessageParams{SessionID: otherSession.ID, ChannelID: otherChannel.ID, AuthorType: "USER", UserID: &f.session.UserID, Content: "Other message"})
	if err != nil {
		t.Fatal(err)
	}
	otherEvaluation, err := f.q.UpsertPendingEvaluation(ctx, otherSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(practice.Item{Original: "Other message", Correction: "Other corrected message", Attempts: []practice.Attempt{}, Context: []string{}})
	if err = f.q.MaterializePracticeItem(ctx, store.MaterializePracticeItemParams{UserID: f.session.UserID, SessionID: otherSession.ID, EvaluationID: otherEvaluation.ID, MessageID: otherMessage.ID, Snapshot: snapshot}); err != nil {
		t.Fatal(err)
	}
	otherItems, err := f.q.ListPracticeItems(ctx, store.ListPracticeItemsParams{SessionID: otherSession.ID, UserID: f.session.UserID})
	if err != nil || len(otherItems) != 1 {
		t.Fatal(otherItems, err)
	}
	if w := req("POST", "/practice/attempts", f.owner, fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":"answer"}]}`, uuid.New(), otherItems[0].ID)); w.Code != 400 {
		t.Fatal("cross-session exercise", w.Code)
	}
	for _, body := range []string{
		`null`, `{}`, `[]`, `true`,
		fmt.Sprintf(`{"requestId":%q,"answers":[]}`, uuid.New()),
		fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":" "}]}`, uuid.New(), id),
		fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":"x"}]}`, uuid.New(), uuid.New()),
		fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":"x"},{"exerciseId":%q,"answer":"x"}]}`, uuid.New(), id, id),
		fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":%q}]}`, uuid.New(), id, strings.Repeat("界", 4001)),
		fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":"x"},{"exerciseId":%q,"answer":"x"},{"exerciseId":%q,"answer":"x"},{"exerciseId":%q,"answer":"x"},{"exerciseId":%q,"answer":"x"},{"exerciseId":%q,"answer":"x"}]}`, uuid.New(), id, id, id, id, id, id),
	} {
		if w := req("POST", "/practice/attempts", f.owner, body); w.Code != 400 {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	validBody := fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":"answer"}]}`, uuid.New(), id)
	for _, body := range []string{validBody + ` {}`, strings.TrimSuffix(validBody, "}") + `,"correction":"client rubric"}`} {
		if w := req("POST", "/practice/attempts", f.owner, body); w.Code != 400 {
			t.Fatalf("strict body accepted: %d", w.Code)
		}
	}
	rid := uuid.New().String()
	body := fmt.Sprintf(`{"requestId":%q,"answers":[{"exerciseId":%q,"answer":"I've finished the update."},{"exerciseId":%q,"answer":"wrong"}]}`, rid, id, queue.Items[1].ID)
	w = req("POST", "/practice/attempts", f.owner, body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result practice.Result
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Attempts) != 2 || result.Attempts[0].Verdict != "ACCEPTED" || result.Attempts[1].Verdict != "NEEDS_WORK" {
		t.Fatal(result)
	}
	if replay := req("POST", "/practice/attempts", f.owner, body); replay.Code != 200 || replay.Body.String() != w.Body.String() {
		t.Fatal("replay changed", replay.Body.String())
	}
	terminal, err := svc.Queue(ctx, f.session.ID, f.session.UserID)
	if err != nil || len(terminal.CompletedBatches) != 1 || terminal.CompletedBatches[0].RequestID != rid || terminal.CompletedBatches[0].Attempts[0].Verdict != "ACCEPTED" {
		t.Fatal("terminal request result missing", terminal, err)
	}
	changed := strings.Replace(body, "I've finished the update.", "A changed answer", 1)
	if different := req("POST", "/practice/attempts", f.owner, changed); different.Code != 409 {
		t.Fatal("changed payload replay", different.Code)
	}
	if p.calls != 1 {
		t.Fatal("duplicate call", p.calls)
	}
	// Concurrent same request is durably claimed once, even by another service instance.
	p.block = make(chan struct{})
	p.entered = make(chan struct{}, 1)
	rid = uuid.New().String()
	in := practice.Submission{RequestID: rid, Answers: []practice.Answer{{ExerciseID: id, Answer: "Equivalent wording"}}}
	done := make(chan error, 1)
	go func() { _, err := svc.Submit(ctx, f.session.ID, f.session.UserID, in); done <- err }()
	<-p.entered
	processing, err := svc.Queue(ctx, f.session.ID, f.session.UserID)
	if err != nil || len(processing.CompletedBatches) != 1 || len(processing.PendingBatches) != 1 || processing.PendingBatches[0].RequestID != rid || len(processing.Items[0].Attempts) != 1 {
		t.Fatal("running claim state", processing, err)
	}
	second := practice.New(f.q, p, "eval-fixture")
	if _, err := second.Submit(ctx, f.session.ID, f.session.UserID, in); !errors.Is(err, practice.ErrConflict) {
		t.Fatal("concurrent claim", err)
	}
	close(p.block)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	p.block = nil
	p.entered = nil
	p.fail = true
	in.RequestID = uuid.New().String()
	in.Answers[0].Answer = "My retained quota answer"
	got, err := svc.Submit(ctx, f.session.ID, f.session.UserID, in)
	if err != nil || got.Attempts[0].Verdict != "UNAVAILABLE" {
		t.Fatal(got, err)
	}
	reloaded, err := second.Queue(ctx, f.session.ID, f.session.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Items[0].Attempts) != 3 || reloaded.Items[0].Attempts[2].Answer != "My retained quota answer" {
		t.Fatal(reloaded.Items[0])
	}
	p.fail = false
	p.malformed = true
	in.RequestID = uuid.New().String()
	got, err = svc.Submit(ctx, f.session.ID, f.session.UserID, in)
	if err != nil || got.Attempts[0].Verdict != "UNAVAILABLE" {
		t.Fatal(got, err)
	}
	// Recovery marks crash-interrupted answers unavailable, without a model call.
	stale := uuid.New()
	_, err = f.q.ClaimPracticeBatch(ctx, store.ClaimPracticeBatchParams{RequestID: stale, UserID: f.session.UserID, SessionID: f.session.ID, Answers: json.RawMessage(fmt.Sprintf(`[{"exerciseId":%q,"answer":"Interrupted answer"}]`, id))})
	if err != nil {
		t.Fatal(err)
	}
	calls := p.calls
	if err = svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = svc.Submit(ctx, f.session.ID, f.session.UserID, practice.Submission{RequestID: stale.String(), Answers: []practice.Answer{{ExerciseID: id, Answer: "Interrupted answer"}}})
	if err != nil || got.Attempts[0].Verdict != "UNAVAILABLE" || p.calls != calls {
		t.Fatal(got, err, p.calls)
	}
	// A disconnected client still receives a durable unavailable checkpoint.
	p.fail = false
	p.malformed = false
	p.block = make(chan struct{})
	p.entered = make(chan struct{}, 1)
	canceled, cancel := context.WithCancel(ctx)
	in.RequestID = uuid.New().String()
	in.Answers[0].Answer = "Canceled retained answer"
	go func() { _, err := svc.Submit(canceled, f.session.ID, f.session.UserID, in); done <- err }()
	<-p.entered
	cancel()
	if err := <-done; err != nil {
		t.Fatal("cleanup did not survive cancellation", err)
	}
	p.block = nil
	p.entered = nil
	canceledReplay, err := svc.Submit(ctx, f.session.ID, f.session.UserID, in)
	if err != nil || canceledReplay.Attempts[0].Verdict != "UNAVAILABLE" || canceledReplay.Attempts[0].Answer != "Canceled retained answer" {
		t.Fatal(canceledReplay, err)
	}
	// SQL owner guard does not claim work for another owner.
	_, err = f.q.ClaimPracticeBatch(ctx, store.ClaimPracticeBatchParams{RequestID: uuid.New(), UserID: uuid.New(), SessionID: f.session.ID, Answers: json.RawMessage(`[]`)})
	if err == nil {
		t.Fatal("cross-owner claim")
	}
}

func practiceRouter(f *evalFixture, s *practice.Service) http.Handler {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Deps{H: &handlers.Handlers{Q: f.q, Log: log, Practice: s}, Hub: ws.NewHub(log, nil), JWT: auth.NewJWT("test-secret", time.Hour), Log: log})
}
