package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
}

func NewOpenAICompat(baseURL, apiKey string, timeout time.Duration) *OpenAICompat {
	return &OpenAICompat{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type chatReq struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
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
	body, err := json.Marshal(chatReq{
		Model: req.Model, Messages: req.Messages, Stream: stream,
		MaxTokens: req.MaxTokens, Temperature: req.Temperature,
	})
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("llm request: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (c *OpenAICompat) Complete(ctx context.Context, req Request) (string, error) {
	resp, err := c.do(ctx, req, false)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out chatResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

func (c *OpenAICompat) Stream(ctx context.Context, req Request, onDelta func(string)) (string, error) {
	resp, err := c.do(ctx, req, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Some proxies ignore stream:true; handle plain JSON too.
	if ct := resp.Header.Get("Content-Type"); strings.HasPrefix(ct, "application/json") {
		var out chatResp
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", err
		}
		if len(out.Choices) == 0 {
			return "", fmt.Errorf("llm returned no choices")
		}
		text := out.Choices[0].Message.Content
		if text != "" {
			onDelta(text)
		}
		return text, nil
	}

	var full strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk chatResp
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil {
			return full.String(), fmt.Errorf("llm stream error: %s", chunk.Error.Message)
		}
		for _, ch := range chunk.Choices {
			if d := ch.Delta.Content; d != "" {
				full.WriteString(d)
				onDelta(d)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return full.String(), fmt.Errorf("read llm stream: %w", err)
	}
	return full.String(), nil
}
