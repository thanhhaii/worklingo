package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/worklingo/backend/db/migrations"
	"github.com/worklingo/backend/internal/auth"
	"github.com/worklingo/backend/internal/chat"
	"github.com/worklingo/backend/internal/config"
	"github.com/worklingo/backend/internal/deadline"
	"github.com/worklingo/backend/internal/evaluation"
	apihttp "github.com/worklingo/backend/internal/http"
	"github.com/worklingo/backend/internal/http/handlers"
	"github.com/worklingo/backend/internal/lifecycle"
	"github.com/worklingo/backend/internal/llm"
	"github.com/worklingo/backend/internal/persona"
	"github.com/worklingo/backend/internal/scenario"
	"github.com/worklingo/backend/internal/session"
	"github.com/worklingo/backend/internal/store"
	"github.com/worklingo/backend/internal/task"
	"github.com/worklingo/backend/internal/validator"
	"github.com/worklingo/backend/internal/workctx"
	"github.com/worklingo/backend/internal/ws"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	migrateOnly := len(os.Args) > 1 && os.Args[1] == "migrate"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = pool.Ping(pctx)
	cancel()
	if err != nil {
		return errors.New("cannot connect to database: " + err.Error())
	}
	if err := migrate(pool); err != nil {
		return err
	}
	if migrateOnly {
		log.Info("migrations applied")
		return nil
	}

	q := store.New(pool)
	cat, err := persona.LoadCatalog()
	if err != nil {
		return err
	}
	if cfg.LLMAPIKey == "" {
		log.Warn("LLM_API_KEY is empty; LLM calls will likely fail")
	}
	provider := llm.NewOpenAICompat(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMTimeout)

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	reg := lifecycle.NewRegistry(appCtx)

	hub := ws.NewHub(log, func(ctx context.Context, userID, sessionID uuid.UUID) bool {
		s, err := q.GetSession(ctx, sessionID)
		return err == nil && s.UserID == userID
	})
	chatSvc := chat.NewService(q, hub)
	val := validator.New(provider, cfg.LLMChatModel, log)
	deadlines := deadline.NewService(q, chatSvc, hub, val, reg, cfg.ResponseTimeout, log)
	engine := persona.NewEngine(cat, q, chatSvc, deadlines, provider, cfg.LLMChatModel, log)
	sched := scenario.New(scenario.Config{FirstDelay: cfg.TaskFirstDelay, MinInterval: cfg.TaskMinInterval, MaxInterval: cfg.TaskMaxInterval},
		q, cat, chatSvc, deadlines, hub, provider, cfg.LLMChatModel, log)
	tasks := task.NewService(q, engine, deadlines, hub, reg, provider, cfg.LLMChatModel, log)
	wctx := workctx.NewGenerator(provider, cfg.LLMChatModel, log)
	evalSvc := evaluation.New(appCtx, q, cat, hub, provider, cfg.LLMEvalModel, log)
	sessions := session.NewManager(pool, q, reg, engine, sched, deadlines, evalSvc, wctx, hub, cfg.SessionDuration, log)

	// Recovery: sessions first (ends expired ones), then deadlines, then evaluations.
	if err := sessions.Recover(ctx); err != nil {
		log.Error("recover sessions", "err", err)
	}
	if err := deadlines.Recover(ctx); err != nil {
		log.Error("recover deadlines", "err", err)
	}
	evalSvc.Recover(ctx)

	jwt := auth.NewJWT(cfg.JWTSecret, cfg.JWTTTL)
	router := apihttp.NewRouter(apihttp.Deps{
		H:   &handlers.Handlers{Q: q, JWT: jwt, Sessions: sessions, Chat: chatSvc, Engine: engine, Deadlines: deadlines, Tasks: tasks, Registry: reg, Log: log},
		Hub: hub, JWT: jwt, CORSOrigin: cfg.CORSOrigin, Log: log,
	})
	srv := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr, "chatModel", cfg.LLMChatModel, "evalModel", cfg.LLMEvalModel,
			"sessionDuration", cfg.SessionDuration.String(), "responseTimeout", cfg.ResponseTimeout.String())
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	_ = srv.Shutdown(sctx)
	sessions.Shutdown()
	deadlines.StopAll()
	appCancel()
	return nil
}

func migrate(pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, ".")
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
