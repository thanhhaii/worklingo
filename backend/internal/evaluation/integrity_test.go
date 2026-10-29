package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/store"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type fixtureProvider struct {
	text  string
	err   error
	calls int
}

func (p *fixtureProvider) Complete(_ context.Context, _ llm.Request) (string, error) {
	p.calls++
	return p.text, p.err
}
func (p *fixtureProvider) Stream(context.Context, llm.Request, func(string)) (string, error) {
	panic("unexpected streaming call")
}
func reviewStatus(t *testing.T, v any) string {
	t.Helper()
	raw, _ := json.Marshal(v)
	var obj map[string]any
	_ = json.Unmarshal(raw, &obj)
	s, _ := obj["reviewStatus"].(string)
	return s
}
func TestMissingAndFailedMessageReviewsAreUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		err        error
		first      string
	}{
		{"omitted message", `{"messages":[{"id":"m1","issues":[]}]}`, nil, "AVAILABLE"},
		{"provider failure", "", errors.New("offline"), "UNAVAILABLE"},
		{"missing issues", `{"messages":[{"id":"m1"}]}`, nil, "UNAVAILABLE"},
		{"malformed", "{", nil, "UNAVAILABLE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m1, m2 := store.Message{ID: uuid.New(), Content: "Hello"}, store.Message{ID: uuid.New(), Content: "Thanks"}
			d := &sessionData{msgs: []store.Message{m1, m2}, learner: []store.Message{m1, m2}}
			s := &Service{llm: &fixtureProvider{text: tc.text, err: tc.err}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			got := s.reviewMessages(context.Background(), d)
			if reviewStatus(t, got[0]) != tc.first || reviewStatus(t, got[1]) != "UNAVAILABLE" {
				t.Fatalf("expected %s / UNAVAILABLE, got %+v", tc.first, got)
			}
		})
	}
}
func TestMissingTaskPointsAreUnavailable(t *testing.T) {
	idx := 0
	got := assembleTaskReview(store.Task{ID: uuid.New()}, []store.TalkingPoint{{ID: uuid.New()}, {ID: uuid.New()}}, []rawPointReview{{Index: &idx, Reason: "reason", Sample: "sample"}}, nil)
	if reviewStatus(t, got) != "UNAVAILABLE" {
		t.Fatalf("incomplete task must be unavailable: %+v", got)
	}
}
func TestLegacyReviewsDoNotBecomeVerifiedClean(t *testing.T) {
	e := store.Evaluation{Status: "DONE", MessageReviews: json.RawMessage(`[{"messageId":"old","issues":[]}]`), TaskReviews: json.RawMessage(`[{"taskId":"old","talkingPoints":[]}]`)}
	got := dto.FromEvaluation(e)
	if reviewStatus(t, got.MessageReviews[0]) != "UNAVAILABLE" || reviewStatus(t, got.TaskReviews[0]) != "UNAVAILABLE" || got.Status == "DONE" {
		t.Fatalf("legacy coverage must be unknown: %+v", got)
	}
}
func TestScoreFailureMakesOnlyOnePaidCall(t *testing.T) {
	p := &fixtureProvider{err: errors.New("offline")}
	s := &Service{llm: p, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_, err := s.score(context.Background(), &sessionData{})
	if err == nil || p.calls != 1 {
		t.Fatalf("got %d calls and %v; want one failed call", p.calls, err)
	}
}
func TestScoreRejectsMissingSummary(t *testing.T) {
	s := &Service{llm: &fixtureProvider{text: `{"grammar":60,"tone":70,"vocabulary":80}`}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	_, err := s.score(context.Background(), &sessionData{})
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		t.Fatal("incomplete score response accepted")
	}
}

func TestDuplicateMessageIDsAreUnavailableEvenWhenOneOmitsIssues(t *testing.T) {
	m := store.Message{ID: uuid.New(), Content: "Hello"}
	d := &sessionData{msgs: []store.Message{m}, learner: []store.Message{m}}
	s := &Service{llm: &fixtureProvider{text: `{"messages":[{"id":"m1"},{"id":"m1","issues":[]}]}`}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if got := s.reviewMessages(context.Background(), d); reviewStatus(t, got[0]) != "UNAVAILABLE" {
		t.Fatalf("duplicate id was verified: %+v", got)
	}
}

func TestTaskReviewsRequireExplicitPointFieldsAndUniqueIDs(t *testing.T) {
	for _, raw := range []string{
		`{"tasks":[{"id":"t1","points":[{"index":0,"reason":"Reason","sample":"Sample"}]}]}`,
		`{"tasks":[{"id":"t1","points":[{"index":0,"learnerQuote":"","reason":"Reason","sample":"Sample"}]},{"id":"t1","points":[{"index":0,"learnerQuote":"","reason":"Reason","sample":"Sample"}]}]}`,
	} {
		task := store.Task{ID: uuid.New()}
		d := &sessionData{tasks: []store.Task{task}, points: map[uuid.UUID][]store.TalkingPoint{task.ID: {{ID: uuid.New()}}}}
		s := &Service{llm: &fixtureProvider{text: raw}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
		if got := s.reviewTasks(context.Background(), d, false); reviewStatus(t, got[0]) != "UNAVAILABLE" {
			t.Fatalf("incomplete or duplicate response was verified: %+v", got)
		}
	}
}

func TestInventedTaskQuoteMakesReviewUnavailable(t *testing.T) {
	idx := 0
	got := assembleTaskReview(store.Task{}, []store.TalkingPoint{{ID: uuid.New()}}, []rawPointReview{{Index: &idx, LearnerQuote: "Invented quote", Reason: "Reason", Sample: "Sample"}}, []string{"Actual learner text"})
	if got.ReviewStatus != "UNAVAILABLE" || got.TalkingPoints[0].LearnerQuote != "" {
		t.Fatalf("invented quote was verified: %+v", got)
	}
}
