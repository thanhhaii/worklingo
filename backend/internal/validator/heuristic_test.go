package validator

import "testing"

func TestHeuristic(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"", false},
		{"   ", false},
		{"👍", false},
		{"👍👍 🎉", false},
		{"ok", false},
		{"OK!", false},
		{"Sure thing 👍", false},
		{"@pm_sarah ok", false},
		{"will do", false},
		{"yes please", false},
		{"Это мой ответ на вопрос", false},
		{"I will finish the user stories by Friday.", true},
		{"@pm_sarah Sure, I'll update the ticket today.", true},
		{"The bug is caused by a race condition.", true},
	}
	for _, c := range cases {
		got := Heuristic(c.in)
		if got.OK != c.ok {
			t.Errorf("Heuristic(%q) = %v (%s), want %v", c.in, got.OK, got.Reason, c.ok)
		}
	}
}
