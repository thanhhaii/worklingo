package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/worklingo/backend/internal/config"
)

func TestCompletionFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantText, wantErr string
		status                        int
	}{
		{"normal", `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`, "hello", "", 200},
		{"json for CompleteJSON", `{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`, `{"ok":true}`, "", 200},
		{"truncated", `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`, "partial", "truncated", 200},
		{"empty", `{"choices":[{"message":{"content":"  "},"finish_reason":"stop"}]}`, "  ", "empty", 200},
		{"no choices", `{"choices":[]}`, "", "no choices", 200},
		{"rate limit", `{"error":{"message":"secret-response-body"}}`, "", "429", 429},
		{"provider error", `{"error":{"message":"secret-response-body"}}`, "", "provider error", 200},
		{"malformed", `{"choices":`, "", "decode", 200},
		{"filtered", `{"choices":[{"message":{"content":"partial"},"finish_reason":"content_filter"}]}`, "partial", "finish", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			p := NewOpenAICompat(server.URL, "test-key", time.Second)
			text, err := p.Complete(context.Background(), Request{Model: "model"})
			checkResult(t, text, err, tc.wantText, tc.wantErr)
		})
	}
}

func TestStreams(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body, wantText, wantErr string }{
		{"sse", "text/event-stream", ": keepalive\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "hello", ""},
		{"json fallback", "application/json; charset=utf-8", `{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`, "hello", ""},
		{"json truncated", "application/json", `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`, "partial", "truncated"},
		{"json error", "application/json", `{"error":{"message":"secret-response-body"}}`, "", "provider error"},
		{"json empty", "application/json", `{"choices":[{"message":{"content":""}}]}`, "", "empty"},
		{"malformed after partial", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: invalid-secret-response-body\n\ndata: [DONE]\n\n", "partial", "decode"},
		{"incomplete", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", "partial", "incomplete"},
		{"length", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n", "partial", "truncated"},
		{"empty", "text/event-stream", "data: [DONE]\n\n", "", "empty"},
		{"error after partial", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: {\"error\":{\"message\":\"secret-response-body\"}}\n\n", "partial", "provider error"},
		{"finish without done", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n", "hello", ""},
		{"multiline data", "text/event-stream", "data: {\"choices\":[\ndata: {\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "hello", ""},
		{"oversized event after partial", "text/event-stream", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\ndata: " + strings.Repeat("x", 1024*1024) + "\n\n", "partial", "read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			p := NewOpenAICompat(server.URL, "test-key", time.Second)
			var deltas strings.Builder
			text, err := p.Stream(context.Background(), Request{Model: "model"}, func(s string) { deltas.WriteString(s) })
			checkResult(t, text, err, tc.wantText, tc.wantErr)
			if deltas.String() != text {
				t.Fatalf("callback received %q, return text %q", deltas.String(), text)
			}
		})
	}
}

func TestFailureIsNotRetried(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "stream"}[stream], func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("secret-response-body"))
			}))
			defer server.Close()
			p := NewOpenAICompat(server.URL, "test-key", time.Second)
			var text string
			var err error
			if stream {
				text, err = p.Stream(context.Background(), Request{Model: "model"}, func(string) {})
			} else {
				text, err = p.Complete(context.Background(), Request{Model: "model"})
			}
			checkResult(t, text, err, "", "429")
			if got := calls.Load(); got != 1 {
				t.Fatalf("got %d upstream calls, want one", got)
			}
		})
	}
}

func TestProviderRedirectErrorDoesNotLeakResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://[secret-response-body")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	p := NewOpenAICompat(server.URL, "test-key", time.Second)
	text, err := p.Complete(context.Background(), Request{Model: "model"})
	checkResult(t, text, err, "", "llm request")
}

func TestGeminiSelectedProfileRequest(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("LLM_PROVIDER", "gemini")
	t.Setenv("LLM_CHAT_PROVIDER", "")
	t.Setenv("LLM_EVAL_PROVIDER", "")
	t.Setenv("GEMINI_API_KEY", "gemini-test-key")
	t.Setenv("GEMINI_CHAT_MODEL", "selected-chat")
	t.Setenv("GEMINI_EVAL_MODEL", "selected-eval")
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete JSON", true: "stream"}[stream], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1beta/openai/chat/completions" || r.Header.Get("Authorization") != "Bearer gemini-test-key" {
					t.Error("wrong selected endpoint or auth")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				wantModel := "selected-eval"
				if stream {
					wantModel = "selected-chat"
				}
				if body["model"] != wantModel || body["reasoning_effort"] != "minimal" || body["stream"] != stream {
					t.Error("wrong selected model, reasoning effort or stream flag")
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"},"finish_reason":"stop"}]}`))
				}
			}))
			defer server.Close()
			t.Setenv("GEMINI_BASE_URL", server.URL+"/v1beta/openai")
			cfg, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			profile, err := cfg.EvalLLM()
			if stream {
				profile, err = cfg.ChatLLM()
			}
			if err != nil {
				t.Fatal(err)
			}
			p := NewOpenAICompat(profile.BaseURL, profile.APIKey, cfg.LLMTimeout)
			p.ReasoningEffort = "minimal"
			if stream {
				text, err := p.Stream(context.Background(), Request{Model: profile.Model}, func(string) {})
				checkResult(t, text, err, "hello", "")
			} else {
				var out struct {
					OK bool `json:"ok"`
				}
				if err := CompleteJSON(context.Background(), p, Request{Model: profile.Model}, &out); err != nil || !out.OK {
					t.Fatalf("JSON result = %v, err = %v", out.OK, err)
				}
			}
		})
	}
}

func checkResult(t *testing.T, text string, err error, wantText, wantErr string) {
	t.Helper()
	if text != wantText {
		t.Fatalf("text = %q, want %q", text, wantText)
	}
	if wantErr == "" && err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)) {
		t.Fatalf("want %q error, got %v", wantErr, err)
	}
	if err != nil && strings.Contains(err.Error(), "secret-response-body") {
		t.Fatal("error leaked provider response body")
	}
}

func TestLegacyRequestContract(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "stream"}[stream], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("wrong request endpoint, method or auth")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing JSON request content type")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "legacy-model" || body["stream"] != stream || body["max_tokens"] != float64(400) {
					t.Error("wrong model, stream or token budget")
				}
				if _, exists := body["reasoning_effort"]; exists {
					t.Error("legacy request contains reasoning_effort")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hello"},"finish_reason":"stop"}]}`))
			}))
			defer server.Close()
			p := NewOpenAICompat(server.URL+"/v1/", "test-key", time.Second)
			req := Request{Model: "legacy-model", MaxTokens: 400, Messages: []Message{{Role: RoleUser, Content: "test"}}}
			var text string
			var err error
			if stream {
				text, err = p.Stream(context.Background(), req, func(string) {})
			} else {
				text, err = p.Complete(context.Background(), req)
			}
			checkResult(t, text, err, "hello", "")
		})
	}
}
