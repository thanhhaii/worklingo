package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// OpenAICompat is a small hand-written client for OpenAI-compatible
// /chat/completions endpoints with SSE streaming.
type OpenAICompat struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// ReasoningEffort is omitted by default to preserve legacy endpoint behavior.
	ReasoningEffort string
	// ProviderName is a safe configuration label, never an endpoint or credential.
	ProviderName string
}

func NewOpenAICompat(baseURL, apiKey string, timeout time.Duration) *OpenAICompat {
	return &OpenAICompat{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		APIKey:       apiKey,
		HTTP:         &http.Client{Timeout: timeout},
		ProviderName: "openai_compat",
	}
}

type tokenUsage struct {
	PromptTokens     *int64 `json:"prompt_tokens"`
	CompletionTokens *int64 `json:"completion_tokens"`
	TotalTokens      *int64 `json:"total_tokens"`
}

type chatReq struct {
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	Stream          bool      `json:"stream"`
	MaxTokens       int       `json:"max_tokens,omitempty"`
	Temperature     *float64  `json:"temperature,omitempty"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
}

type chatResp struct {
	Usage   *tokenUsage `json:"usage"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *OpenAICompat) do(ctx context.Context, req Request, stream bool) (*http.Response, error) {
	wire := chatReq{
		Model: req.Model, Messages: req.Messages, Stream: stream,
		MaxTokens: req.MaxTokens, Temperature: req.Temperature,
		ReasoningEffort: c.ReasoningEffort,
	}
	if stream {
		wire.StreamOptions = &struct {
			IncludeUsage bool `json:"include_usage"`
		}{true}
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("llm request: invalid endpoint configuration")
	}
	hr.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		hr.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	if stream {
		hr.Header.Set("Accept", "text/event-stream")
	}
	resp, err := c.HTTP.Do(hr)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("llm request: %w", ctx.Err())
		}
		// Transport errors may contain URLs returned in provider redirects.
		return nil, errors.New("llm request failed: transport or timeout error")
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		// Provider bodies can echo prompts or credentials; callers log errors.
		return nil, fmt.Errorf("llm http %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *OpenAICompat) Complete(ctx context.Context, req Request) (text string, err error) {
	start := time.Now()
	var usage *tokenUsage
	defer func() { c.logCall(ctx, req, false, start, err, usage) }()
	resp, err := c.do(ctx, req, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return decodeCompletion(resp, &usage)
}

func decodeCompletion(resp *http.Response, usage **tokenUsage) (string, error) {
	var out chatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", errors.New("decode llm response: invalid or incomplete JSON")
	}
	*usage = out.Usage
	if out.Error != nil {
		return "", errors.New("llm provider error")
	}
	if len(out.Choices) == 0 {
		return "", errors.New("llm returned no choices")
	}
	text := out.Choices[0].Message.Content
	if err := finishError(out.Choices[0].FinishReason); err != nil {
		return text, err
	}
	if strings.TrimSpace(text) == "" {
		return text, errors.New("llm returned empty completion")
	}
	return text, nil
}

func finishError(reason string) error {
	switch reason {
	case "", "stop":
		return nil
	case "length":
		return errors.New("llm completion truncated: output token limit reached")
	default:
		return errors.New("llm completion failed: unsupported finish reason")
	}
}

func (c *OpenAICompat) Stream(ctx context.Context, req Request, onDelta func(string)) (text string, err error) {
	start := time.Now()
	var usage *tokenUsage
	defer func() { c.logCall(ctx, req, true, start, err, usage) }()
	resp, err := c.do(ctx, req, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Some proxies ignore stream:true; handle plain JSON too.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/json") {
		text, err := decodeCompletion(resp, &usage)
		if text != "" {
			onDelta(text)
		}
		return text, err
	}

	var full strings.Builder
	var data []string
	finished, done := false, false
	consume := func() error {
		if len(data) == 0 {
			return nil
		}
		payload := strings.Join(data, "\n")
		data = nil
		if payload == "[DONE]" {
			done = true
			return nil
		}
		var chunk chatResp
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return errors.New("decode llm stream: invalid or incomplete JSON event")
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.Error != nil {
			return errors.New("llm stream provider error")
		}
		if len(chunk.Choices) == 0 {
			return nil
		} // usage-only events
		ch := chunk.Choices[0]
		if d := ch.Delta.Content; d != "" {
			full.WriteString(d)
			onDelta(d)
		}
		if err := finishError(ch.FinishReason); err != nil {
			return err
		}
		if ch.FinishReason == "stop" {
			finished = true
		}
		return nil
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			if err := consume(); err != nil {
				return full.String(), err
			}
			if done {
				break
			}
		} else if strings.HasPrefix(line, "data:") {
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := sc.Err(); err != nil {
		return full.String(), errors.New("read llm stream failed")
	}
	if err := consume(); err != nil {
		return full.String(), err
	}
	if !done && !finished {
		return full.String(), errors.New("llm stream incomplete: missing completion marker")
	}
	if strings.TrimSpace(full.String()) == "" {
		return full.String(), errors.New("llm returned empty completion")
	}
	return full.String(), nil
}

// logCall records only operational metadata. Missing provider counts stay null.
func (c *OpenAICompat) logCall(ctx context.Context, req Request, stream bool, start time.Time, err error, usage *tokenUsage) {
	status := "success"
	if err != nil {
		status = "error"
	}
	count := func(p *int64) any {
		if p == nil {
			return nil
		}
		return *p
	}
	if usage == nil {
		usage = &tokenUsage{}
	}
	slog.InfoContext(ctx, "llm.call", "provider", c.ProviderName, "model", req.Model,
		"stream", stream, "status", status, "duration_ms", time.Since(start).Milliseconds(),
		"prompt_tokens", count(usage.PromptTokens), "completion_tokens", count(usage.CompletionTokens), "total_tokens", count(usage.TotalTokens))
}
