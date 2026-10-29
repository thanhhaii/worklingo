package practice

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/store"
	"strings"
	"testing"
)

type fakeJudge struct {
	calls    int
	response string
	err      error
	request  llm.Request
}

func (p *fakeJudge) Complete(_ context.Context, r llm.Request) (string, error) {
	p.calls++
	p.request = r
	return p.response, p.err
}
func (*fakeJudge) Stream(context.Context, llm.Request, func(string)) (string, error) { panic("stream") }
func TestExtractOnlyAvailableValidIssuesWithBoundedContext(t *testing.T) {
	mid, ch := uuid.New(), uuid.New()
	msgs := []store.Message{{ID: uuid.New(), ChannelID: ch, Content: strings.Repeat("x", 2000)}, {ID: uuid.New(), ChannelID: ch, Content: "Relevant context"}, {ID: mid, AuthorType: "USER", ChannelID: ch, Content: "I has finished."}}
	reviews := []dto.MessageReview{{MessageID: mid.String(), ReviewStatus: "AVAILABLE", Content: "I has finished.", Issues: []dto.MessageIssue{{Original: "has", Corrected: "have", Explanation: "Chủ ngữ I", Category: "GRAMMAR"}, {Original: "missing", Corrected: "x", Explanation: "x"}}}, {MessageID: mid.String(), ReviewStatus: "UNAVAILABLE", Issues: []dto.MessageIssue{{Original: "has", Corrected: "have", Explanation: "x"}}}}
	got := extract(reviews, msgs)
	if len(got) != 1 || got[0].Original != "I has finished." || got[0].Correction != "I have finished." || len(got[0].Context) != 2 || len([]rune(got[0].Context[0])) > 1000 {
		t.Fatalf("bad extraction: %+v", got)
	}
}
func TestJudgeBatchAcceptsEquivalentWordingWithOneCall(t *testing.T) {
	p := &fakeJudge{response: `{"results":[{"exerciseId":"one","verdict":"ACCEPTED","feedback":"Câu tương đương, phù hợp."},{"exerciseId":"two","verdict":"NEEDS_WORK","feedback":"Cần sửa ngữ pháp."}]}`}
	s := &Service{provider: p, model: "evaluation-model"}
	items := []Item{{ID: "one", Original: "I has finished.", Correction: "I have finished.", Target: "has", Explanation: "Chủ ngữ I", Context: []string{"Status?"}}, {ID: "two", Original: "You must do now", Correction: "Could you do this now?"}}
	got := s.judge(context.Background(), items, []Answer{{"one", "I've completed it."}, {"two", "Do now"}}, nil)
	if p.calls != 1 || len(got) != 2 || got[0].Verdict != "ACCEPTED" || got[1].Verdict != "NEEDS_WORK" || p.request.Model != "evaluation-model" || !strings.Contains(p.request.Messages[0].Content, "Status?") {
		t.Fatalf("%+v calls=%d", got, p.calls)
	}
}
func TestInvalidOrUnavailableJudgeNeverMarksAnswersWrong(t *testing.T) {
	for _, response := range []string{`{}`, `{"results":null}`, `{"results":[null]}`, `{"results":[{"exerciseId":"one","verdict":"ACCEPTED","feedback":null}]}`, `{"results":[{"exerciseId":"one","verdict":"MAYBE","feedback":"Cần xem lại."}]}`, `{"results":[{"exerciseId":"one","verdict":"ACCEPTED"}]}`, `{"results":[{"exerciseId":"other","verdict":"ACCEPTED","feedback":"OK"}]}`, `{"results":[{"exerciseId":"one","verdict":"ACCEPTED","feedback":"OK"},{"exerciseId":"one","verdict":"ACCEPTED","feedback":"OK"}]}`, `broken`} {
		p := &fakeJudge{response: response}
		s := &Service{provider: p}
		got := s.judge(context.Background(), []Item{{ID: "one"}}, []Answer{{"one", "answer"}}, nil)
		if len(got) != 1 || got[0].Verdict != "UNAVAILABLE" || got[0].Answer != "answer" {
			t.Fatalf("%s: %+v", response, got)
		}
	}
	p := &fakeJudge{err: errors.New("429 secret provider details")}
	s := &Service{provider: p}
	got := s.judge(context.Background(), []Item{{ID: "one"}}, []Answer{{"one", "answer"}}, nil)
	if got[0].Verdict != "UNAVAILABLE" || strings.Contains(got[0].Feedback, "secret") {
		t.Fatal(got)
	}
}

func TestThreadContextExcludesUnrelatedChannelsAndThreads(t *testing.T) {
	ch, other, parent, mid, thread := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	msgs := []store.Message{
		{ID: parent, ChannelID: ch, Content: "Can you send the status?"},
		{ID: uuid.New(), ChannelID: ch, ParentID: &parent, Content: "Keep the blocker clear."},
		{ID: uuid.New(), ChannelID: other, Content: "Private unrelated message"},
		{ID: uuid.New(), ChannelID: ch, ParentID: &thread, Content: "Another thread"},
		{ID: mid, ChannelID: ch, ParentID: &parent, AuthorType: "USER", Content: "I has finished."},
	}
	reviews := []dto.MessageReview{{MessageID: mid.String(), Content: "I has finished.", ReviewStatus: "AVAILABLE", Issues: []dto.MessageIssue{{Original: "has", Corrected: "have", Explanation: "Sửa chủ ngữ."}}}}
	got := extract(reviews, msgs)
	if len(got) != 1 || len(got[0].Context) != 2 || got[0].Context[0] != "Can you send the status?" || got[0].Context[1] != "Keep the blocker clear." {
		t.Fatal(got)
	}
}
