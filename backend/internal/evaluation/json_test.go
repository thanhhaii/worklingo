package evaluation

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/worklingo/backend/internal/store"
)

// Exercise the actual scoring/review paths: accepting only a valid prefix must
// never turn a malformed complete response into persisted successful coverage.
func TestEvaluationWholeResponseJSON(t *testing.T) {
	for _, component := range []string{"score", "messages", "tasks"} {
		t.Run(component, func(t *testing.T) {
			valid := map[string]string{
				"score":    `{"grammar":60,"tone":70,"vocabulary":80,"overall":70,"summary":"Good attempt."}`,
				"messages": `{"messages":[{"id":"m1","issues":[]}]}`,
				"tasks":    `{"tasks":[{"id":"t1","points":[{"index":0,"learnerQuote":"","reason":"Reason","sample":"Sample"}]}]}`,
			}[component]
			for _, tc := range []struct {
				name, envelope string
				available      bool
			}{
				{"plain", " \n%s\t", true},
				{"json fence", "```json\n%s\n```", true},
				{"bare fence", "\n```\n%s\n```\n", true},
				{"CRLF fence", "```json\r\n%s\r\n```\r\n", true},
				{"truncated second object", "%s {\"private-suffix\":", false},
				{"trailing garbage", "%s private-suffix", false},
				{"second JSON value", "%s {}", false},
				{"leading prose", "private-prefix %s", false},
				{"trailing prose", "%s That is the result.", false},
				{"fence with leading prose", "private-prefix\n```json\n%s\n```", false},
				{"fence with trailing garbage", "```json\n%s\n``` private-suffix", false},
				{"fence with trailing value", "```json\n%s\n``` {}", false},
				{"malformed inside fence", "```json\n%s {\"private-suffix\":\n```", false},
				{"unclosed fence", "```json\n%s", false},
				{"unsupported fence", "```javascript\n%s\n```", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					p := &fixtureProvider{text: fmt.Sprintf(tc.envelope, valid)}
					s := &Service{llm: p, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
					switch component {
					case "score":
						result, err := s.score(context.Background(), &sessionData{})
						if (err == nil) != tc.available {
							t.Errorf("available=%v, score=%+v, err=%v", tc.available, result, err)
						}
						if err != nil && (strings.Contains(err.Error(), "private-prefix") || strings.Contains(err.Error(), "private-suffix")) {
							t.Error("decode error exposed response content")
						}
						if tc.available && (result.Grammar == nil || *result.Grammar != 60 || result.Summary != "Good attempt.") {
							t.Errorf("valid score changed: %+v", result)
						}
					case "messages":
						m := store.Message{ID: uuid.New(), Content: "Hello"}
						d := &sessionData{msgs: []store.Message{m}, learner: []store.Message{m}}
						got := s.reviewMessages(context.Background(), d)
						if len(got) != 1 || (got[0].ReviewStatus == "AVAILABLE") != tc.available || len(got[0].Issues) != 0 {
							t.Errorf("available=%v, message reviews=%+v", tc.available, got)
						}
					case "tasks":
						task := store.Task{ID: uuid.New()}
						d := &sessionData{tasks: []store.Task{task}, points: map[uuid.UUID][]store.TalkingPoint{task.ID: {{ID: uuid.New()}}}}
						got := s.reviewTasks(context.Background(), d, false)
						if len(got) != 1 || (got[0].ReviewStatus == "AVAILABLE") != tc.available {
							t.Errorf("available=%v, task reviews=%+v", tc.available, got)
						}
						if tc.available && (len(got[0].TalkingPoints) != 1 || got[0].TalkingPoints[0].Sample != "Sample") {
							t.Errorf("valid task feedback changed: %+v", got)
						}
					}
					if p.calls != 1 {
						t.Errorf("got %d provider calls, want one", p.calls)
					}
				})
			}
		})
	}
}
