// Package llm provides a minimal provider abstraction over OpenAI-compatible chat APIs.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string
	Messages    []Message
	MaxTokens   int
	Temperature *float64
}

// Provider is implemented by LLM backends.
type Provider interface {
	// Stream calls onDelta for each content chunk and returns the full text.
	Stream(ctx context.Context, req Request, onDelta func(string)) (string, error)
	// Complete returns the full response text.
	Complete(ctx context.Context, req Request) (string, error)
}

// ExtractJSON pulls a JSON object/array out of an LLM response, stripping
// markdown fences and surrounding prose.
func ExtractJSON(s string) (string, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		// skip language tag line
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			tag := strings.TrimSpace(rest[:nl])
			if tag == "" || !strings.ContainsAny(tag, "{[") {
				rest = rest[nl+1:]
			}
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		s = strings.TrimSpace(rest)
	}
	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return "", errors.New("no JSON found in response")
	}
	open := s[start]
	closeCh := byte('}')
	if open == '[' {
		closeCh = ']'
	}
	// scan for matching bracket, respecting strings
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", errors.New("unterminated JSON in response")
}

// CompleteJSON calls Complete and unmarshals the extracted JSON into out.
func CompleteJSON(ctx context.Context, p Provider, req Request, out any) error {
	text, err := p.Complete(ctx, req)
	if err != nil {
		return err
	}
	raw, err := ExtractJSON(text)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), out)
}
