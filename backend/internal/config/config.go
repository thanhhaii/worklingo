// Package config loads application configuration from the environment.
package config

import (
	"fmt"
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

	LLMBaseURL   string        `env:"LLM_BASE_URL" envDefault:"http://127.0.0.1:20128/v1"`
	LLMAPIKey    string        `env:"LLM_API_KEY"`
	LLMChatModel string        `env:"LLM_CHAT_MODEL" envDefault:"cc/claude-haiku-4-5-20251001"`
	LLMEvalModel string        `env:"LLM_EVAL_MODEL" envDefault:"cc/claude-sonnet-5-5"`
	LLMTimeout   time.Duration `env:"LLM_TIMEOUT" envDefault:"120s"`

	SessionDuration time.Duration `env:"SESSION_DURATION" envDefault:"60m"`
	ResponseTimeout time.Duration `env:"RESPONSE_TIMEOUT" envDefault:"5m"`
	TaskFirstDelay  time.Duration `env:"TASK_FIRST_DELAY" envDefault:"2m"`
	TaskMinInterval time.Duration `env:"TASK_MIN_INTERVAL" envDefault:"6m"`
	TaskMaxInterval time.Duration `env:"TASK_MAX_INTERVAL" envDefault:"12m"`
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
	return &c, nil
}
