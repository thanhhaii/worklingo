package evaluation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/store"
)

const messageReviewPrompt = `You are an expert Business English coach reviewing a learner's Slack messages from a simulated workplace at a software company. The learner is a non-native speaker (Vietnamese).
Below are numbered LEARNER messages (m1, m2, ...), each with a little preceding context (for understanding only; never review the context).
For EACH learner message list ALL real issues: grammar, spelling, punctuation, awkward or unnatural phrasing, wrong tone for a workplace (too blunt, too casual, impolite), and weak or wrong vocabulary. Do not invent issues; a correct message has an empty list. Do not nitpick purely stylistic preferences.
For every issue:
- "original": the EXACT text copied from the learner message (a substring, character for character).
- "corrected": the corrected English replacing that original text.
- "explanation": a short explanation written in VIETNAMESE.
- "category": "GRAMMAR", "TONE" or "VOCABULARY".
- "severity": "MAJOR" if it changes the meaning, is hard to understand, or is rude/unprofessional; otherwise "MINOR".
Respond with ONLY JSON, including every message id:
{"messages":[{"id":"m1","issues":[{"original":"...","corrected":"...","explanation":"...","category":"GRAMMAR","severity":"MINOR"}]}]}

Messages:
%s`

const taskReviewPrompt = `You are an expert Business English coach reviewing how a learner handled "tasks" (requests from colleagues) in a simulated workplace Slack. The learner is a non-native speaker (Vietnamese).
Each task has numbered talking points (in Vietnamese): content the learner had to get across. For each point you get its status (COVERED or PENDING) from the system, and the learner's messages that came after the task.
For EVERY point of EVERY task return:
- "learnerQuote": the EXACT text (a substring, character for character) from the learner's messages that covered the point, or their best attempt at it; "" if nothing relevant was written.
- "reason": one or two sentences in VIETNAMESE explaining why it was covered, or what was missing or wrong.
- "sample": ONE natural English sentence a professional would write to convey this point (written in English).
Respond with ONLY JSON:
{"tasks":[{"id":"t1","points":[{"index":0,"learnerQuote":"...","reason":"...","sample":"..."}]}]}

Tasks:
%s`

// reviewMessages produces one MessageReview per learner message. Failed batches
// fall back to messages without issues.
func (s *Service) reviewMessages(ctx context.Context, d *sessionData) []dto.MessageReview {
	issues := map[uuid.UUID][]rawIssue{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, rng := range batches(len(d.learner), reviewBatchSize) {
		batch := d.learner[rng[0]:rng[1]]
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := s.reviewBatch(ctx, d, batch)
			if err != nil {
				s.log.Warn("message review batch failed, no issues recorded", "messages", len(batch), "err", err)
				return
			}
			mu.Lock()
			for id, is := range got {
				issues[id] = is
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return assembleMessageReviews(d.learner, d.labels, issues)
}

func (s *Service) reviewBatch(ctx context.Context, d *sessionData, batch []store.Message) (map[uuid.UUID][]rawIssue, error) {
	idx := map[uuid.UUID]int{}
	for i, m := range d.msgs {
		idx[m.ID] = i
	}
	byKey := map[string]uuid.UUID{}
	var sb strings.Builder
	for i, m := range batch {
		key := fmt.Sprintf("m%d", i+1)
		byKey[key] = m.ID
		for _, c := range d.contextBefore(idx[m.ID], 2) {
			fmt.Fprintf(&sb, "  (context) %s\n", oneLine(c, 300))
		}
		fmt.Fprintf(&sb, "%s [%s]: %s\n\n", key, d.labels[m.ChannelID], m.Content)
	}
	var out struct {
		Messages []struct {
			ID     string     `json:"id"`
			Issues []rawIssue `json:"issues"`
		} `json:"messages"`
	}
	cctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	temp := 0.1
	if err := llm.CompleteJSON(cctx, s.llm, llm.Request{Model: s.model, MaxTokens: 8000, Temperature: &temp,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf(messageReviewPrompt, strings.TrimSpace(sb.String()))}}}, &out); err != nil {
		return nil, err
	}
	res := map[uuid.UUID][]rawIssue{}
	for _, m := range out.Messages {
		if id, ok := byKey[strings.ToLower(strings.TrimSpace(m.ID))]; ok {
			res[id] = m.Issues
		}
	}
	return res, nil
}

// contextBefore returns up to n preceding messages in the same channel as msgs[i].
func (d *sessionData) contextBefore(i, n int) []string {
	ch := d.msgs[i].ChannelID
	var out []string
	for j := i - 1; j >= 0 && len(out) < n; j-- {
		if d.msgs[j].ChannelID != ch {
			continue
		}
		who := "LEARNER"
		if d.msgs[j].AuthorType == chat.AuthorPersona {
			who = "colleague"
		}
		out = append([]string{who + ": " + d.msgs[j].Content}, out...)
	}
	return out
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "..."
	}
	return s
}

// learnerTextsFor returns the learner messages a task could have been answered by:
// same channel, after the task, and (if in a thread) in the task's thread.
func learnerTextsFor(t store.Task, learner []store.Message) []store.Message {
	var out []store.Message
	for _, m := range learner {
		if m.ChannelID != t.ChannelID || !m.CreatedAt.After(t.CreatedAt) {
			continue
		}
		if m.ParentID != nil && *m.ParentID != t.MessageID {
			continue
		}
		out = append(out, m)
	}
	return out
}

// reviewTasks produces one TaskReview per task with one LLM call over all tasks.
// With skipLLM (or on failure) quote/reason/sample stay empty.
func (s *Service) reviewTasks(ctx context.Context, d *sessionData, skipLLM bool) []dto.TaskReview {
	if len(d.tasks) == 0 {
		return []dto.TaskReview{}
	}
	var raw map[int][]rawPointReview
	texts := make([][]string, len(d.tasks))
	var sb strings.Builder
	for ti, t := range d.tasks {
		fmt.Fprintf(&sb, "Task t%d: %s\n", ti+1, t.Title)
		if req, ok := d.byID(t.MessageID); ok {
			fmt.Fprintf(&sb, "Colleague's request: %s\n", oneLine(req.Content, 600))
		}
		fmt.Fprintf(&sb, "Talking points:\n")
		for i, p := range d.points[t.ID] {
			fmt.Fprintf(&sb, "  %d. [%s] %s\n", i, p.Status, p.Content)
		}
		fmt.Fprintf(&sb, "Learner messages after the task:\n")
		ms := learnerTextsFor(t, d.learner)
		if len(ms) > 15 {
			ms = ms[:15]
		}
		for _, m := range ms {
			texts[ti] = append(texts[ti], m.Content)
			fmt.Fprintf(&sb, "  - %s\n", oneLine(m.Content, 600))
		}
		if len(ms) == 0 {
			fmt.Fprintf(&sb, "  (none)\n")
		}
		sb.WriteString("\n")
	}
	if !skipLLM {
		var out struct {
			Tasks []struct {
				ID     string           `json:"id"`
				Points []rawPointReview `json:"points"`
			} `json:"tasks"`
		}
		cctx, cancel := context.WithTimeout(ctx, 150*time.Second)
		temp := 0.3
		err := llm.CompleteJSON(cctx, s.llm, llm.Request{Model: s.model, MaxTokens: 6000, Temperature: &temp,
			Messages: []llm.Message{{Role: llm.RoleUser, Content: fmt.Sprintf(taskReviewPrompt, strings.TrimSpace(sb.String()))}}}, &out)
		cancel()
		if err != nil {
			s.log.Warn("task review failed, using empty reviews", "err", err)
		} else {
			raw = map[int][]rawPointReview{}
			for _, t := range out.Tasks {
				var n int
				if _, err := fmt.Sscanf(strings.ToLower(strings.TrimSpace(t.ID)), "t%d", &n); err == nil && n >= 1 && n <= len(d.tasks) {
					raw[n-1] = t.Points
				}
			}
		}
	}
	reviews := make([]dto.TaskReview, 0, len(d.tasks))
	for ti, t := range d.tasks {
		reviews = append(reviews, assembleTaskReview(t, d.points[t.ID], raw[ti], texts[ti]))
	}
	return reviews
}

func (d *sessionData) byID(id uuid.UUID) (store.Message, bool) {
	for _, m := range d.msgs {
		if m.ID == id {
			return m, true
		}
	}
	return store.Message{}, false
}
