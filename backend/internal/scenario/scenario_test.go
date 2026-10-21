package scenario

import (
	"reflect"
	"testing"
)

func TestNormalisePoints(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"trim and drop empty", []string{"  a ", "", "   ", "b"}, []string{"a", "b"}},
		{"clamp to three", []string{"a", "b", "c", "d"}, []string{"a", "b", "c"}},
		{"fallback from title", nil, []string{"Trả lời yêu cầu: Fix bug"}},
		{"all blank", []string{" ", ""}, []string{"Trả lời yêu cầu: Fix bug"}},
	}
	for _, c := range cases {
		if got := normalisePoints(c.in, " Fix bug "); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestFallbacksHavePoints(t *testing.T) {
	for _, f := range fallbacks {
		n := len(f.Points)
		if n < 1 || n > 3 || (f.Priority == "CRITICAL" && n < 2) {
			t.Errorf("fallback %q has %d points", f.Title, n)
		}
	}
}
