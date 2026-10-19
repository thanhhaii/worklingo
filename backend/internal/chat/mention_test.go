package chat

import (
	"reflect"
	"testing"
)

func TestParseMentions(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"hey @pm_sarah can you check?", []string{"pm_sarah"}},
		{"@Alex and @david, thoughts?", []string{"alex", "david"}},
		{"@alex @alex @ALEX", []string{"alex"}},
		{"email me at bob@example.com", nil},
		{"Thanks @john.doe.", []string{"john.doe"}},
		{"no mentions here", nil},
		{"(@tech_lead_alex) ping", []string{"tech_lead_alex"}},
	}
	for _, c := range cases {
		got := ParseMentions(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseMentions(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	if !Mentions("Hi @Bob_99, are you there?", "bob_99") {
		t.Error("expected mention of bob_99")
	}
	if Mentions("Hi @bob_990", "bob_99") {
		t.Error("unexpected prefix match")
	}
}
