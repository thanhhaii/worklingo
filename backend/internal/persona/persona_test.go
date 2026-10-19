package persona

import (
	"strings"
	"testing"
)

func TestCatalog(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"sarah": "pm_sarah", "alex": "tech_lead_alex", "david": "client_david"}
	for id, handle := range want {
		p, ok := c.Get(id)
		if !ok || p.Handle != handle || p.SystemPrompt == "" {
			t.Errorf("persona %s: %+v", id, p)
		}
		for _, tok := range []string{id, handle} {
			if got, ok := c.ByMention(tok); !ok || got != id {
				t.Errorf("ByMention(%q) = %q", tok, got)
			}
		}
	}
	sp := c.SystemPrompt(c.byID["alex"], "maria", "#incident-room")
	if !strings.Contains(sp, "@maria") || strings.Contains(sp, "{{") {
		t.Errorf("system prompt not rendered: %s", sp)
	}
	if !NeedsResponse("@maria can you check the logs?", "maria") || NeedsResponse("thanks @maria", "maria") {
		t.Error("NeedsResponse heuristic wrong")
	}
}
