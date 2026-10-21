// Package persona contains the static AI personas and the reply engine.
package persona

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/worklingo/backend/internal/dto"
)

//go:embed prompts/*.yaml
var promptFS embed.FS

type Persona struct {
	ID              string   `yaml:"id"`
	Handle          string   `yaml:"handle"`
	DisplayName     string   `yaml:"displayName"`
	Role            string   `yaml:"role"`
	AvatarColor     string   `yaml:"avatarColor"`
	DefaultChannels []string `yaml:"defaultChannels"`
	Keywords        []string `yaml:"keywords"`
	Greeting        string   `yaml:"greeting"`
	SystemPrompt    string   `yaml:"systemPrompt"`
	TaskIdeas       []string `yaml:"taskIdeas"`
}

func (p Persona) DTO() dto.Persona {
	return dto.Persona{ID: p.ID, Handle: p.Handle, DisplayName: p.DisplayName, Role: p.Role, AvatarColor: p.AvatarColor}
}

// Order is the canonical persona order.
var Order = []string{"sarah", "alex", "david"}

// Catalog holds the loaded personas and shared rules.
type Catalog struct {
	byID  map[string]Persona
	rules string
}

func LoadCatalog() (*Catalog, error) {
	c := &Catalog{byID: map[string]Persona{}}
	var common struct {
		Rules string `yaml:"rules"`
	}
	b, err := promptFS.ReadFile("prompts/common.yaml")
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &common); err != nil {
		return nil, fmt.Errorf("common.yaml: %w", err)
	}
	c.rules = common.Rules
	for _, id := range Order {
		b, err := promptFS.ReadFile("prompts/" + id + ".yaml")
		if err != nil {
			return nil, err
		}
		var p Persona
		if err := yaml.Unmarshal(b, &p); err != nil {
			return nil, fmt.Errorf("%s.yaml: %w", id, err)
		}
		if p.ID != id {
			return nil, fmt.Errorf("%s.yaml: id mismatch %q", id, p.ID)
		}
		c.byID[id] = p
	}
	return c, nil
}

func (c *Catalog) Get(id string) (Persona, bool) {
	p, ok := c.byID[id]
	return p, ok
}

func (c *Catalog) All() []Persona {
	out := make([]Persona, 0, len(Order))
	for _, id := range Order {
		out = append(out, c.byID[id])
	}
	return out
}

func (c *Catalog) DTOs() []dto.Persona {
	out := []dto.Persona{}
	for _, p := range c.All() {
		out = append(out, p.DTO())
	}
	return out
}

// ByMention resolves a mention token (handle, id, or first name) to a persona id.
func (c *Catalog) ByMention(token string) (string, bool) {
	token = strings.ToLower(token)
	for _, p := range c.All() {
		first := strings.ToLower(strings.Fields(p.DisplayName)[0])
		if token == p.Handle || token == p.ID || token == first {
			return p.ID, true
		}
	}
	return "", false
}

// SystemPrompt builds the full system prompt for persona p. A non-nil work
// context is appended as a shared-facts block so all personas stay consistent.
func (c *Catalog) SystemPrompt(p Persona, username, channelLabel string, wc *dto.WorkContext) string {
	var others []string
	for _, o := range c.All() {
		if o.ID != p.ID {
			others = append(others, fmt.Sprintf("@%s (%s, %s)", o.Handle, o.DisplayName, o.Role))
		}
	}
	sort.Strings(others)
	r := strings.NewReplacer("{{username}}", username, "{{channel}}", channelLabel, "{{others}}", strings.Join(others, ", "))
	out := strings.TrimSpace(p.SystemPrompt) + "\n\n" + r.Replace(c.rules)
	if wc != nil {
		out += "\n\nShared work context (the learner's current work; stay consistent with it, never contradict it; the facts are written in Vietnamese but you always reply in English):\n" + wc.Describe()
	}
	return out
}

// Render replaces {{username}} in a template.
func Render(tmpl, username string) string {
	return strings.TrimSpace(strings.ReplaceAll(tmpl, "{{username}}", username))
}
