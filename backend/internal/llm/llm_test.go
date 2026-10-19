package llm

import "testing"

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                                    `{"a":1}`,
		"```json\n{\"a\":1}\n```":                    `{"a":1}`,
		"```\n{\"a\":\"x}\"}\n```":                   `{"a":"x}"}`,
		"Sure! Here it is: {\"a\":{\"b\":2}} thanks": `{"a":{"b":2}}`,
		"[1,2,3]":                           "[1,2,3]",
		`{"s":"quote \" and } brace"} tail`: `{"s":"quote \" and } brace"}`,
	}
	for in, want := range cases {
		got, err := ExtractJSON(in)
		if err != nil {
			t.Fatalf("ExtractJSON(%q) error: %v", in, err)
		}
		if got != want {
			t.Errorf("ExtractJSON(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ExtractJSON("no json here"); err == nil {
		t.Error("expected error for missing JSON")
	}
	if _, err := ExtractJSON(`{"a":1`); err == nil {
		t.Error("expected error for unterminated JSON")
	}
}
