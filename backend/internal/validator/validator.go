package validator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/worklingo/backend/internal/llm"
)

// Result of validating a reply.
type Result struct {
	Valid            bool   `json:"valid"`
	AddressesRequest bool   `json:"addressesRequest"`
	IsEnglish        bool   `json:"isEnglish"`
	Reason           string `json:"reason"`
}

type Validator struct {
	llm   llm.Provider
	model string
	log   *slog.Logger
}

func New(p llm.Provider, model string, log *slog.Logger) *Validator {
	return &Validator{llm: p, model: model, log: log}
}

const systemPrompt = `You check replies in a workplace chat used for Business English practice.
Given a REQUEST (a message from a colleague asking something) and a REPLY from the learner, decide:
- addressesRequest: does the reply meaningfully respond to the request (answers, commits to action, asks a relevant clarifying question, or politely pushes back with a reason)? Generic filler like "ok" or off-topic text is false.
- isEnglish: is the reply written in English (minor mistakes are fine)?
Respond with ONLY a JSON object: {"addressesRequest": bool, "isEnglish": bool, "reason": "short explanation"}`

// Validate runs the heuristic first, then asks the LLM. If the LLM is
// unavailable the heuristic result is used.
func (v *Validator) Validate(ctx context.Context, request, reply string) Result {
	h := Heuristic(reply)
	if !h.OK {
		return Result{Valid: false, Reason: h.Reason}
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var out struct {
		AddressesRequest *bool  `json:"addressesRequest"`
		IsEnglish        *bool  `json:"isEnglish"`
		Reason           string `json:"reason"`
	}
	temp := 0.0
	err := llm.CompleteJSON(ctx, v.llm, llm.Request{
		Model:       v.model,
		MaxTokens:   200,
		Temperature: &temp,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: fmt.Sprintf("REQUEST:\n%s\n\nREPLY:\n%s", request, reply)},
		},
	}, &out)
	if err != nil || out.AddressesRequest == nil || out.IsEnglish == nil {
		v.log.Warn("validator llm failed, using heuristic", "err", err)
		return Result{Valid: true, AddressesRequest: true, IsEnglish: true, Reason: "heuristic pass (llm unavailable)"}
	}
	return Result{
		Valid:            *out.AddressesRequest && *out.IsEnglish,
		AddressesRequest: *out.AddressesRequest,
		IsEnglish:        *out.IsEnglish,
		Reason:           out.Reason,
	}
}
