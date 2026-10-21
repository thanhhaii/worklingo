package evaluation

import (
	"strings"

	"github.com/google/uuid"

	"github.com/worklingo/backend/internal/dto"
	"github.com/worklingo/backend/internal/store"
)

const reviewBatchSize = 40

// rawIssue is one issue as returned by the LLM.
type rawIssue struct {
	Original    string `json:"original"`
	Corrected   string `json:"corrected"`
	Explanation string `json:"explanation"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
}

// filterIssues keeps issues whose original is an exact substring of content,
// normalising category and severity.
func filterIssues(content string, in []rawIssue) []dto.MessageIssue {
	out := []dto.MessageIssue{}
	for _, i := range in {
		orig := strings.TrimSpace(i.Original)
		if orig == "" || !strings.Contains(content, orig) {
			continue
		}
		cat := strings.ToUpper(strings.TrimSpace(i.Category))
		switch cat {
		case "GRAMMAR", "TONE", "VOCABULARY":
		default:
			cat = "GRAMMAR"
		}
		sev := strings.ToUpper(strings.TrimSpace(i.Severity))
		if sev != "MAJOR" {
			sev = "MINOR"
		}
		out = append(out, dto.MessageIssue{
			Original: orig, Corrected: strings.TrimSpace(i.Corrected),
			Explanation: strings.TrimSpace(i.Explanation), Category: cat, Severity: sev,
		})
	}
	return out
}

// batches splits n items into consecutive [start,end) ranges of at most size.
func batches(n, size int) [][2]int {
	var out [][2]int
	for s := 0; s < n; s += size {
		out = append(out, [2]int{s, min(s+size, n)})
	}
	return out
}

// channelLabels maps channel ids to "#name" or "DM: <Persona DisplayName>".
func channelLabels(chs []store.Channel, personaName func(id string) (string, bool)) map[uuid.UUID]string {
	out := map[uuid.UUID]string{}
	for _, c := range chs {
		if c.Kind == "DM" {
			name := c.Name
			if c.PersonaID != nil {
				if n, ok := personaName(*c.PersonaID); ok {
					name = n
				}
			}
			out[c.ID] = "DM: " + name
		} else {
			out[c.ID] = "#" + c.Name
		}
	}
	return out
}

// assembleMessageReviews builds one review per learner message (chronological as
// given). issues is keyed by message id; missing = no issues.
func assembleMessageReviews(msgs []store.Message, labels map[uuid.UUID]string, issues map[uuid.UUID][]rawIssue) []dto.MessageReview {
	out := make([]dto.MessageReview, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, dto.MessageReview{
			MessageID: m.ID.String(), ChannelLabel: labels[m.ChannelID], Content: m.Content,
			CreatedAt: m.CreatedAt.UTC(), Issues: filterIssues(m.Content, issues[m.ID]),
		})
	}
	return out
}

// rawPointReview is the LLM's review of one talking point.
type rawPointReview struct {
	Index        *int   `json:"index"`
	LearnerQuote string `json:"learnerQuote"`
	Reason       string `json:"reason"`
	Sample       string `json:"sample"`
}

// assembleTaskReview builds a task review. covered comes from the DB; the quote
// must be an exact substring of one of the learner texts, else it is dropped.
// raw may be nil (soft failure): fields are then empty.
func assembleTaskReview(t store.Task, pts []store.TalkingPoint, raw []rawPointReview, learnerTexts []string) dto.TaskReview {
	byIdx := map[int]rawPointReview{}
	for _, r := range raw {
		if r.Index != nil {
			byIdx[*r.Index] = r
		}
	}
	out := dto.TaskReview{
		TaskID: t.ID.String(), Title: t.Title, PersonaID: t.PersonaID, Status: t.Status,
		TalkingPoints: make([]dto.TalkingPointReview, 0, len(pts)),
	}
	for i, p := range pts {
		r := byIdx[i]
		quote := strings.TrimSpace(r.LearnerQuote)
		if !inAny(quote, learnerTexts) {
			quote = ""
		}
		reason := strings.TrimSpace(r.Reason)
		if reason == "" {
			reason = p.Reason
		}
		out.TalkingPoints = append(out.TalkingPoints, dto.TalkingPointReview{
			TalkingPointID: p.ID.String(), Content: p.Content, Covered: p.Status == "COVERED",
			LearnerQuote: quote, Reason: reason, Sample: strings.TrimSpace(r.Sample),
		})
	}
	return out
}

func inAny(s string, texts []string) bool {
	if s == "" {
		return false
	}
	for _, t := range texts {
		if strings.Contains(t, s) {
			return true
		}
	}
	return false
}
