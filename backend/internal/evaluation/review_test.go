package evaluation

import (
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/store"
)

func TestFilterIssues(t *testing.T) {
	content := "I am agree with your plan. We deploy tomorrow."
	got := filterIssues(content, []rawIssue{
		{Original: "I am agree", Corrected: "I agree", Explanation: "x", Category: "grammar", Severity: "major"},
		{Original: "not in message", Corrected: "y"},
		{Original: "  ", Corrected: "z"},
		{Original: "We deploy", Corrected: "We will deploy", Category: "weird", Severity: ""},
	})
	if len(got) != 2 {
		t.Fatalf("want 2 issues, got %+v", got)
	}
	if got[0].Category != "GRAMMAR" || got[0].Severity != "MAJOR" {
		t.Errorf("normalisation: %+v", got[0])
	}
	if got[1].Category != "GRAMMAR" || got[1].Severity != "MINOR" {
		t.Errorf("defaults: %+v", got[1])
	}
	if filterIssues("hi", nil) == nil {
		t.Error("must return empty slice, not nil")
	}
}

func TestBatches(t *testing.T) {
	if got := batches(85, 40); !reflect.DeepEqual(got, [][2]int{{0, 40}, {40, 80}, {80, 85}}) {
		t.Errorf("got %v", got)
	}
	if got := batches(0, 40); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestChannelLabels(t *testing.T) {
	pid := "sarah"
	gen, dm := uuid.New(), uuid.New()
	got := channelLabels([]store.Channel{
		{ID: gen, Kind: "CHANNEL", Name: "general"},
		{ID: dm, Kind: "DM", Name: "sarah", PersonaID: &pid},
	}, func(id string) (string, bool) { return "Sarah Chen", id == "sarah" })
	if got[gen] != "#general" || got[dm] != "DM: Sarah Chen" {
		t.Errorf("labels: %v", got)
	}
}

func TestAssembleMessageReviews(t *testing.T) {
	ch := uuid.New()
	m1 := store.Message{ID: uuid.New(), ChannelID: ch, Content: "I am agree", CreatedAt: time.Now()}
	m2 := store.Message{ID: uuid.New(), ChannelID: ch, Content: "Sounds good", CreatedAt: time.Now()}
	got := assembleMessageReviews([]store.Message{m1, m2}, map[uuid.UUID]string{ch: "#general"},
		map[uuid.UUID][]rawIssue{m1.ID: {{Original: "I am agree", Corrected: "I agree"}, {Original: "bogus"}}})
	if len(got) != 2 || got[0].ChannelLabel != "#general" || len(got[0].Issues) != 1 {
		t.Fatalf("got %+v", got)
	}
	if got[1].Issues == nil || len(got[1].Issues) != 0 {
		t.Errorf("message without issues must have empty non-nil slice: %+v", got[1])
	}
}

func TestAssembleTaskReview(t *testing.T) {
	task := store.Task{ID: uuid.New(), Title: "T", PersonaID: "alex", Status: "ACKNOWLEDGED"}
	p0 := store.TalkingPoint{ID: uuid.New(), Content: "a", Status: "COVERED", Reason: "db reason"}
	p1 := store.TalkingPoint{ID: uuid.New(), Content: "b", Status: "PENDING"}
	zero := 0
	raw := []rawPointReview{{Index: &zero, LearnerQuote: "I fixed it", Reason: "", Sample: " Sample. "}}
	r := assembleTaskReview(task, []store.TalkingPoint{p0, p1}, raw, []string{"Yes, I fixed it today"})
	if !r.TalkingPoints[0].Covered || r.TalkingPoints[1].Covered {
		t.Error("covered must come from DB status")
	}
	if r.TalkingPoints[0].LearnerQuote != "I fixed it" || r.TalkingPoints[0].Reason != "db reason" || r.TalkingPoints[0].Sample != "Sample." {
		t.Errorf("point 0: %+v", r.TalkingPoints[0])
	}
	r = assembleTaskReview(task, []store.TalkingPoint{p0}, []rawPointReview{{Index: &zero, LearnerQuote: "made up"}}, []string{"real text"})
	if r.TalkingPoints[0].LearnerQuote != "" {
		t.Error("quote not in learner text must be dropped")
	}
	r = assembleTaskReview(task, []store.TalkingPoint{p1}, nil, nil)
	if len(r.TalkingPoints) != 1 || r.TalkingPoints[0].Sample != "" {
		t.Errorf("soft failure: %+v", r)
	}
}
