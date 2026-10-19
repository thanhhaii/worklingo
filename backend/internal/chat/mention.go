package chat

import (
	"regexp"
	"strings"
)

var mentionRe = regexp.MustCompile(`(?:^|[^\w@])@([A-Za-z0-9_][A-Za-z0-9_.\-]*)`)

// ParseMentions returns the lowercase handles mentioned with @ in content,
// deduplicated, in order of appearance. Trailing dots/dashes are trimmed.
func ParseMentions(content string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range mentionRe.FindAllStringSubmatch(content, -1) {
		h := strings.ToLower(strings.TrimRight(m[1], ".-"))
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

// Mentions reports whether content @mentions the given handle (case-insensitive).
func Mentions(content, handle string) bool {
	handle = strings.ToLower(handle)
	for _, h := range ParseMentions(content) {
		if h == handle {
			return true
		}
	}
	return false
}
