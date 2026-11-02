package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/worklingo/backend/internal/llm"
)

// completeJSON validates the entire evaluation response before exposing results.
// Only a full bare/json fence may surround the JSON; the shared legacy extractor
// intentionally remains more permissive for other LLM consumers.
func (s *Service) completeJSON(ctx context.Context, req llm.Request, out any) error {
	text, err := s.llm.Complete(ctx, req)
	if err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		opener, body, ok := strings.Cut(text, "\n")
		opener = strings.TrimSuffix(opener, "\r")
		closingLine := strings.LastIndexByte(body, '\n')
		if !ok || (opener != "```" && opener != "```json") || closingLine < 0 || body[closingLine+1:] != "```" {
			return errors.New("invalid evaluation JSON fence envelope")
		}
		text = body[:closingLine]
	}
	// Unmarshal checks the whole input, rejecting extra values and truncated or
	// garbage suffixes. Keep decode errors free of provider response contents.
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return errors.New("invalid or incomplete evaluation JSON response")
	}
	return nil
}
