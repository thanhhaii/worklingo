// Package validator decides whether a user reply is a valid response to a request.
package validator

import (
	"strings"
	"unicode"
)

// MinWords is the minimum number of words for a reply to count.
const MinWords = 3

var lowEffort = map[string]bool{
	"ok": true, "okay": true, "k": true, "kk": true, "sure": true, "yes": true, "no": true,
	"yep": true, "nope": true, "thanks": true, "thx": true, "ty": true, "lol": true,
	"noted": true, "fine": true, "got it": true, "will do": true, "on it": true,
	"ok thanks": true, "okay thanks": true, "sure thing": true, "np": true, "done": true,
	"hi": true, "hello": true, "hey": true, "ok will do": true,
}

// HeuristicResult explains a heuristic decision.
type HeuristicResult struct {
	OK     bool
	Reason string
}

// Heuristic performs cheap checks: not empty, not just emoji, not a
// low-effort acknowledgement, enough words, mostly Latin letters.
func Heuristic(content string) HeuristicResult {
	text := strings.TrimSpace(content)
	if text == "" {
		return HeuristicResult{false, "empty reply"}
	}
	letters, latin := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if r < 0x250 { // Basic Latin + Latin-1 + Latin Extended-A/B
				latin++
			}
		}
	}
	if letters == 0 {
		return HeuristicResult{false, "reply contains no words (emoji or symbols only)"}
	}
	if float64(latin)/float64(letters) < 0.8 {
		return HeuristicResult{false, "reply does not look like English"}
	}
	norm := normalize(text)
	if lowEffort[norm] {
		return HeuristicResult{false, "reply is a low-effort acknowledgement"}
	}
	if n := len(strings.Fields(norm)); n < MinWords {
		return HeuristicResult{false, "reply is too short"}
	}
	return HeuristicResult{true, ""}
}

// normalize lowercases, strips @mentions, punctuation and emoji, and collapses spaces.
func normalize(s string) string {
	var words []string
	for _, w := range strings.Fields(strings.ToLower(s)) {
		if strings.HasPrefix(w, "@") {
			continue
		}
		w = strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' {
				return r
			}
			return -1
		}, w)
		if w != "" {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}
