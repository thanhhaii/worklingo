package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestModelCallUsageLogging(t *testing.T) {
	for _, tc := range []struct {
		name, body                string
		stream, failed, httpError bool
		known                     bool
	}{
		{name: "complete", body: `{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`, known: true},
		{name: "truncated with usage", failed: true, known: true, body: `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}],"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`},
		{name: "malformed", failed: true, body: `{"choices":`},
		{name: "unknown", body: `{"choices":[{"message":{"content":"hello"}}]}`},
		{name: "stream", stream: true, known: true, body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":4,\"total_tokens\":16}}\n\ndata: [DONE]\n\n"},
		{name: "failure", failed: true, httpError: true, body: "private-provider-body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
			defer slog.SetDefault(old)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				_ = json.NewDecoder(r.Body).Decode(&req)
				if tc.stream {
					opts, _ := req["stream_options"].(map[string]any)
					if opts["include_usage"] != true {
						t.Error("stream does not request usage")
					}
					w.Header().Set("Content-Type", "text/event-stream")
				}
				if tc.httpError {
					w.WriteHeader(429)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			p := NewOpenAICompat(srv.URL, "private-key", time.Second)
			p.ProviderName = "gemini"
			req := Request{Model: "test-model", Messages: []Message{{Role: RoleUser, Content: "private-prompt"}}}
			if tc.stream {
				_, _ = p.Stream(context.Background(), req, func(string) {})
			} else {
				_, _ = p.Complete(context.Background(), req)
			}
			var got map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
				t.Fatalf("expected one structured call log, got %q: %v", buf.String(), err)
			}
			want := "success"
			if tc.failed {
				want = "error"
			}
			if got["status"] != want || got["model"] != "test-model" || got["provider"] != "gemini" || got["duration_ms"] == nil {
				t.Fatalf("missing call metadata: %v", got)
			}
			if tc.known {
				if got["prompt_tokens"] != float64(12) || got["completion_tokens"] != float64(4) || got["total_tokens"] != float64(16) {
					t.Fatalf("usage lost: %v", got)
				}
			} else if got["total_tokens"] != nil {
				t.Fatalf("unknown usage fabricated: %v", got)
			}
			for _, secret := range []string{"private-key", "private-prompt", "private-provider-body"} {
				if strings.Contains(buf.String(), secret) {
					t.Fatal("log leaked sensitive text")
				}
			}
		})
	}
}
