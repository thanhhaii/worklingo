// Package config loads application configuration from the environment.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Port        string        `env:"PORT" envDefault:"8080"`
	DatabaseURL string        `env:"DATABASE_URL" envDefault:"postgres://worklingo:worklingo@localhost:5433/worklingo?sslmode=disable"`
	JWTSecret   string        `env:"JWT_SECRET" envDefault:"dev-insecure-secret-change-me"`
	JWTTTL      time.Duration `env:"JWT_TTL" envDefault:"168h"`
	CORSOrigin  string        `env:"CORS_ORIGIN" envDefault:"http://localhost:3000"`
	LogLevel    string        `env:"LOG_LEVEL" envDefault:"info"`

	LLMBaseURL      string        `env:"LLM_BASE_URL" envDefault:"http://127.0.0.1:20128/v1"`
	LLMAPIKey       string        `env:"LLM_API_KEY"`
	LLMChatModel    string        `env:"LLM_CHAT_MODEL" envDefault:"cc/claude-haiku-4-5-20251001"`
	LLMEvalModel    string        `env:"LLM_EVAL_MODEL" envDefault:"cc/claude-sonnet-5-5"`
	LLMTimeout      time.Duration `env:"LLM_TIMEOUT" envDefault:"120s"`
	LLMProvider     string        `env:"LLM_PROVIDER" envDefault:"openai"`
	LLMChatProvider string        `env:"LLM_CHAT_PROVIDER"`
	LLMEvalProvider string        `env:"LLM_EVAL_PROVIDER"`
	GeminiBaseURL   string        `env:"GEMINI_BASE_URL" envDefault:"https://generativelanguage.googleapis.com/v1beta/openai"`
	GeminiAPIKey    string        `env:"GEMINI_API_KEY"`
	GeminiChatModel string        `env:"GEMINI_CHAT_MODEL" envDefault:"gemini-3.1-flash-lite"`
	GeminiEvalModel string        `env:"GEMINI_EVAL_MODEL" envDefault:"gemini-3.1-flash-lite"`

	SessionDuration time.Duration `env:"SESSION_DURATION" envDefault:"60m"`
	ResponseTimeout time.Duration `env:"RESPONSE_TIMEOUT" envDefault:"5m"`
	TaskFirstDelay  time.Duration `env:"TASK_FIRST_DELAY" envDefault:"2m"`
	TaskMinInterval time.Duration `env:"TASK_MIN_INTERVAL" envDefault:"6m"`
	TaskMaxInterval time.Duration `env:"TASK_MAX_INTERVAL" envDefault:"12m"`
}

// LLMProfile contains the selected backend settings for one role.
type LLMProfile struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
}

func (c Config) ChatLLM() (LLMProfile, error) { return c.llmProfile(true) }
func (c Config) EvalLLM() (LLMProfile, error) { return c.llmProfile(false) }

func (c Config) llmProfile(chat bool) (LLMProfile, error) {
	provider := strings.ToLower(strings.TrimSpace(c.LLMProvider))
	if provider == "" {
		provider = "openai"
	}
	if provider != "openai" && provider != "gemini" {
		return LLMProfile{}, fmt.Errorf("LLM_PROVIDER must be openai or gemini")
	}
	selector, name := c.LLMEvalProvider, "LLM_EVAL_PROVIDER"
	if chat {
		selector, name = c.LLMChatProvider, "LLM_CHAT_PROVIDER"
	}
	if selected := strings.ToLower(strings.TrimSpace(selector)); selected != "" {
		provider = selected
	}
	var profile LLMProfile
	switch provider {
	case "openai":
		model := c.LLMEvalModel
		if chat {
			model = c.LLMChatModel
		}
		profile = LLMProfile{provider, c.LLMBaseURL, c.LLMAPIKey, model}
	case "gemini":
		model := c.GeminiEvalModel
		if chat {
			model = c.GeminiChatModel
		}
		profile = LLMProfile{provider, strings.TrimRight(strings.TrimSpace(c.GeminiBaseURL), "/"), strings.TrimSpace(c.GeminiAPIKey), strings.TrimSpace(model)}
		if profile.APIKey == "" {
			return LLMProfile{}, fmt.Errorf("%s selects gemini: set GEMINI_API_KEY in the backend environment", name)
		}
	default:
		return LLMProfile{}, fmt.Errorf("%s must be openai or gemini", name)
	}
	return profile, nil
}

// Load reads .env (if present) and then the process environment.
func Load() (*Config, error) {
	_ = godotenv.Load() // optional; real env vars take precedence
	var c Config
	if err := env.Parse(&c); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	if c.TaskMaxInterval < c.TaskMinInterval {
		c.TaskMaxInterval = c.TaskMinInterval
	}
	if _, err := c.ChatLLM(); err != nil {
		return nil, err
	}
	if _, err := c.EvalLLM(); err != nil {
		return nil, err
	}
	return &c, nil
}
