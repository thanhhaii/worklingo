package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/worklingo/backend/internal/config"
	"github.com/worklingo/backend/internal/llm"
)

func TestNewLLMProviderSendsReasoningOnlyForGemini(t *testing.T) {
	for _, provider := range []string{"openai", "gemini"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				effort, exists := body["reasoning_effort"]
				if provider == "gemini" && effort != "minimal" {
					t.Error("Gemini must send minimal reasoning")
				}
				if provider == "openai" && exists {
					t.Error("legacy endpoint must omit reasoning_effort")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`))
			}))
			defer server.Close()
			p := newLLMProvider(config.LLMProfile{Provider: provider, BaseURL: server.URL, APIKey: "test-key", Model: "model"}, time.Second)
			if p.ProviderName != provider {
				t.Fatalf("call logging label = %s, want %s", p.ProviderName, provider)
			}
			if _, err := p.Complete(context.Background(), llm.Request{Model: "model"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
