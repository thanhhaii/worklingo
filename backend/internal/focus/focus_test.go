package focus

import (
	"strings"
	"testing"
	"time"
)

func TestDefinitions(t *testing.T) {
	for _, g := range []string{"STATUS_UPDATE", "CLARIFY_REQUIREMENTS", "REQUEST_EXTENSION", "POLITE_PUSHBACK", "INCIDENT_UPDATE"} {
		if !Valid(g) || len(Templates(g)) != 3 || !strings.Contains(Instructions(g), g) {
			t.Fatalf("missing %s", g)
		}
		c := Context(g)
		if c.Project == "" || len(c.Notes) == 0 {
			t.Fatal(g)
		}
		for _, v := range Templates(g) {
			if v.Message == "" || !strings.Contains(v.Message, "{{username}}") || len(v.Points) == 0 || v.Channel == "" || v.PersonaID == "" {
				t.Fatalf("invalid %+v", v)
			}
		}
	}
	if Valid("GRAMMAR") {
		t.Fatal("unknown valid")
	}
	for _, d := range []time.Duration{2 * time.Second, 10 * time.Minute, 20 * time.Minute, time.Hour} {
		want := 3
		if d <= 10*time.Minute {
			want = 2
		}
		if TaskLimit(d) != want {
			t.Fatal(d)
		}
	}
}

func TestFactsAndAbsoluteSchedule(t *testing.T) {
	cases := map[string][]string{"STATUS_UPDATE": {"70%", "security rules", "Friday"}, "CLARIFY_REQUIREMENTS": {"40%", "columns", "date range"}, "REQUEST_EXTENSION": {"60%", "Monday", "Wednesday", "Production credentials"}, "POLITE_PUSHBACK": {"70%", "Friday", "audit export", "role checks"}, "INCIDENT_UPDATE": {"8%", "10:42 UTC", "11:15 UTC", "suspected, not confirmed"}}
	for goal, facts := range cases {
		text := Context(goal).Describe()
		for _, fact := range facts {
			if !strings.Contains(text, fact) {
				t.Fatalf("%s missing %s", goal, fact)
			}
		}
		for _, tpl := range Templates(goal) {
			if strings.Contains(strings.ToLower(tpl.Message), "resolved") {
				t.Fatal("invented transition")
			}
			switch tpl.PersonaID {
			case "sarah", "alex", "david":
			default:
				t.Fatal("invalid persona")
			}
			switch tpl.Channel {
			case "general", "sprint-planning", "incident-room":
			default:
				t.Fatal("invalid channel")
			}
		}
	}
	start := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	expiry := start.Add(10 * time.Minute)
	if got := Due(start, expiry, 0); !got.Equal(start.Add(1500 * time.Millisecond)) {
		t.Fatal(got)
	}
	if got := Due(start, expiry, 1); !got.Equal(start.Add(300750 * time.Millisecond)) {
		t.Fatal(got)
	}
	if FirstOffset(2*time.Second) != 100*time.Millisecond {
		t.Fatal("short config timing changed")
	}
}
