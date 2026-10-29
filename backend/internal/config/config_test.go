package config

import (
	"strings"
	"testing"
)

func cleanLLMEnv(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir()) // Load must not read a developer's .env.
	for _, key := range []string{"LLM_PROVIDER", "LLM_CHAT_PROVIDER", "LLM_EVAL_PROVIDER", "LLM_BASE_URL", "LLM_API_KEY", "LLM_CHAT_MODEL", "LLM_EVAL_MODEL", "LLM_TIMEOUT", "GEMINI_BASE_URL", "GEMINI_API_KEY", "GEMINI_CHAT_MODEL", "GEMINI_EVAL_MODEL"} {
		t.Setenv(key, "")
	}
}

func TestLoadRejectsInvalidSelectedProvider(t *testing.T) {
	for _, tc := range []struct {
		name, key, value, want string
	}{
		{"global", "LLM_PROVIDER", "unsupported", "LLM_PROVIDER"},
		{"chat", "LLM_CHAT_PROVIDER", "unsupported", "LLM_CHAT_PROVIDER"},
		{"eval", "LLM_EVAL_PROVIDER", "unsupported", "LLM_EVAL_PROVIDER"},
		{"gemini requires key", "LLM_PROVIDER", "gemini", "GEMINI_API_KEY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanLLMEnv(t)
			t.Setenv("LLM_API_KEY", "legacy-secret-never-print")
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want actionable %s configuration error, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "legacy-secret-never-print") {
				t.Fatal("configuration error exposed a secret")
			}
		})
	}
}

func TestProviderProfiles(t *testing.T) {
	for _, tc := range []struct {
		name       string
		env        map[string]string
		chat, eval LLMProfile
	}{
		{
			name: "legacy defaults",
			chat: LLMProfile{"openai", "http://127.0.0.1:20128/v1", "", "cc/claude-haiku-4-5-20251001"},
			eval: LLMProfile{"openai", "http://127.0.0.1:20128/v1", "", "cc/claude-sonnet-5-5"},
		},
		{
			name: "legacy custom values",
			env:  map[string]string{"LLM_BASE_URL": "https://legacy.example/v1", "LLM_API_KEY": "legacy-test-key", "LLM_CHAT_MODEL": "legacy-chat", "LLM_EVAL_MODEL": "legacy-eval"},
			chat: LLMProfile{"openai", "https://legacy.example/v1", "legacy-test-key", "legacy-chat"},
			eval: LLMProfile{"openai", "https://legacy.example/v1", "legacy-test-key", "legacy-eval"},
		},
		{
			name: "gemini defaults isolated from legacy",
			env:  map[string]string{"LLM_PROVIDER": "gemini", "GEMINI_API_KEY": "gemini-test-key", "LLM_BASE_URL": "https://legacy.example/v1", "LLM_API_KEY": "legacy-test-key", "LLM_CHAT_MODEL": "legacy-chat", "LLM_EVAL_MODEL": "legacy-eval"},
			chat: LLMProfile{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-test-key", "gemini-3.1-flash-lite"},
			eval: LLMProfile{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-test-key", "gemini-3.1-flash-lite"},
		},
		{
			name: "chat overrides global with normalized selectors",
			env:  map[string]string{"LLM_PROVIDER": " GeMiNi ", "LLM_CHAT_PROVIDER": " OPENAI ", "GEMINI_API_KEY": "gemini-test-key"},
			chat: LLMProfile{"openai", "http://127.0.0.1:20128/v1", "", "cc/claude-haiku-4-5-20251001"},
			eval: LLMProfile{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-test-key", "gemini-3.1-flash-lite"},
		},
		{
			name: "eval overrides global and gemini custom fields",
			env:  map[string]string{"LLM_PROVIDER": "openai", "LLM_EVAL_PROVIDER": " GEMINI ", "GEMINI_API_KEY": " gemini-test-key ", "GEMINI_BASE_URL": " https://gemini.example/openai/ ", "GEMINI_CHAT_MODEL": " chat-model ", "GEMINI_EVAL_MODEL": " eval-model "},
			chat: LLMProfile{"openai", "http://127.0.0.1:20128/v1", "", "cc/claude-haiku-4-5-20251001"},
			eval: LLMProfile{"gemini", "https://gemini.example/openai", "gemini-test-key", "eval-model"},
		},
		{
			name: "blank overrides inherit global",
			env:  map[string]string{"LLM_PROVIDER": "gemini", "LLM_CHAT_PROVIDER": " \t", "LLM_EVAL_PROVIDER": " ", "GEMINI_API_KEY": "gemini-test-key", "GEMINI_CHAT_MODEL": "custom-chat", "GEMINI_EVAL_MODEL": "custom-eval"},
			chat: LLMProfile{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-test-key", "custom-chat"},
			eval: LLMProfile{"gemini", "https://generativelanguage.googleapis.com/v1beta/openai", "gemini-test-key", "custom-eval"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanLLMEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			chat, err := cfg.ChatLLM()
			if err != nil {
				t.Fatal(err)
			}
			eval, err := cfg.EvalLLM()
			if err != nil {
				t.Fatal(err)
			}
			if chat != tc.chat || eval != tc.eval {
				t.Fatal("resolved profiles differ from expected provider-specific fields")
			}
		})
	}
}

func TestWhitespaceGeminiKeyIsMissing(t *testing.T) {
	cleanLLMEnv(t)
	t.Setenv("LLM_CHAT_PROVIDER", "gemini")
	t.Setenv("GEMINI_API_KEY", " \t ")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "GEMINI_API_KEY") {
		t.Fatalf("want missing Gemini key error, got %v", err)
	}
}
