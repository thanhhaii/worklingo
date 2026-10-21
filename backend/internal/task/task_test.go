package task

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/store"
)

func TestSelectCandidates(t *testing.T) {
	ch, other := uuid.New(), uuid.New()
	base := time.Now().Add(-time.Hour)
	mk := func(chID uuid.UUID, min int, status string) store.Task {
		return store.Task{ID: uuid.New(), ChannelID: chID, MessageID: uuid.New(), Status: status, CreatedAt: base.Add(time.Duration(min) * time.Minute)}
	}
	t1, t2, t3 := mk(ch, 1, "OPEN"), mk(ch, 2, "ACKNOWLEDGED"), mk(ch, 3, "OPEN")
	done := mk(ch, 4, "DONE")
	elsewhere := mk(other, 5, "OPEN")
	late := mk(ch, 50, "OPEN")
	noPending := mk(ch, 6, "OPEN")
	tasks := []store.Task{t1, t2, t3, done, elsewhere, late, noPending}
	pending := map[uuid.UUID]bool{}
	for _, tk := range tasks {
		pending[tk.ID] = true
	}
	delete(pending, noPending.ID)
	msg := store.Message{ChannelID: ch, CreatedAt: base.Add(10 * time.Minute)}

	got := SelectCandidates(tasks, pending, msg)
	if len(got) != 2 || got[0].ID != t2.ID || got[1].ID != t3.ID {
		t.Fatalf("top-level: want the 2 most recent eligible tasks, got %v", got)
	}

	thread := msg
	thread.ParentID = &t1.MessageID
	got = SelectCandidates(tasks, pending, thread)
	if len(got) != 1 || got[0].ID != t1.ID {
		t.Fatalf("thread: want only the parent's task, got %v", got)
	}

	unrelated := uuid.New()
	thread.ParentID = &unrelated
	if got = SelectCandidates(tasks, pending, thread); len(got) != 0 {
		t.Fatalf("unrelated thread: want none, got %v", got)
	}
}

func TestFollowUpInstruction(t *testing.T) {
	s := FollowUpInstruction("maria", "Estimate story", []store.TalkingPoint{{Content: "Nói về rủi ro"}})
	if !strings.Contains(s, "@maria") || !strings.Contains(s, "Nói về rủi ro") || !strings.Contains(s, "checklist") {
		t.Errorf("unexpected instruction: %s", s)
	}
}
