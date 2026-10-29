package session

import (
	"encoding/json"
	"testing"
	"time"
)

func TestResolveOptions(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Duration
		bad  bool
	}{{`{}`, 2 * time.Second, false}, {`{"learningGoal":"STATUS_UPDATE"}`, 2 * time.Second, false}, {`{"durationMinutes":10}`, 10 * time.Minute, false}, {`{"durationMinutes":20}`, 20 * time.Minute, false}, {`{"durationMinutes":60}`, time.Hour, false}, {`{"durationMinutes":null}`, 0, true}, {`{"learningGoal":null}`, 0, true}, {`{"durationMinutes":5}`, 0, true}, {`{"durationMinutes":"10"}`, 0, true}, {`{"learningGoal":"GRAMMAR"}`, 0, true}} {
		t.Run(tc.raw, func(t *testing.T) {
			var o CreateOptions
			err := json.Unmarshal([]byte(tc.raw), &o)
			var d time.Duration
			if err == nil {
				d, err = ResolveOptions(o, 2*time.Second)
			}
			if (err != nil) != tc.bad || !tc.bad && d != tc.want {
				t.Fatalf("duration=%v err=%v", d, err)
			}
		})
	}
}
