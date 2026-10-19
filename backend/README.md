# WorkLingo backend (AI-Solo)

Go API + WebSocket server for the WorkLingo Slack-like Business English simulator.
The API/WS contract is in [`../docs/CONTRACT.md`](../docs/CONTRACT.md).

Stack: chi, coder/websocket, pgx/v5 + sqlc, goose (embedded migrations, auto-applied on startup),
a small OpenAI-compatible streaming client (SSE), slog, caarlos0/env + godotenv, golang-jwt, bcrypt.

## Run

```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
docker compose -f ../docker-compose.yml up -d   # Postgres on localhost:5433
cp .env.example .env                            # then set LLM_API_KEY and JWT_SECRET
make run                                        # migrations run automatically, listens on :8080
```

Other targets:

| target | what |
|---|---|
| `make migrate` | apply migrations and exit (`go run ./cmd/api migrate`) |
| `make sqlc` | regenerate `internal/store` from `db/queries/*.sql` (install with `make tools`) |
| `make test` / `make vet` | unit tests / vet |
| `make build` | binary at `bin/api` |
| `make smoke` | end-to-end test against a running server (see below) |

## Configuration (env vars, `.env` is loaded if present)

| var | default |
|---|---|
| `PORT` | `8080` |
| `DATABASE_URL` | `postgres://worklingo:worklingo@localhost:5433/worklingo?sslmode=disable` |
| `JWT_SECRET` | insecure dev value — **set it** |
| `JWT_TTL` | `168h` |
| `CORS_ORIGIN` | `http://localhost:3000` (also used as the allowed WS origin) |
| `LOG_LEVEL` | `info` |
| `LLM_BASE_URL` | `http://127.0.0.1:20128/v1` (any OpenAI-compatible `/chat/completions`) |
| `LLM_API_KEY` | – (sent as `Authorization: Bearer …`) |
| `LLM_CHAT_MODEL` | `cc/claude-haiku-4-5-20251001` — persona replies, responder choice, reply validation, task generation |
| `LLM_EVAL_MODEL` | `cc/claude-sonnet-5-5` — post-session evaluation |
| `LLM_TIMEOUT` | `120s` (HTTP timeout per LLM call) |
| `SESSION_DURATION` | `60m` |
| `RESPONSE_TIMEOUT` | `5m` (deadline length) |
| `TASK_FIRST_DELAY` | `2m` |
| `TASK_MIN_INTERVAL` / `TASK_MAX_INTERVAL` | `6m` / `12m` |

Durations use Go syntax (`30s`, `5m`, `1h`).

### Switching models

Set `LLM_CHAT_MODEL` and/or `LLM_EVAL_MODEL` (and `LLM_BASE_URL`/`LLM_API_KEY` for another provider), then restart.
For example, to evaluate with Haiku too: `LLM_EVAL_MODEL=cc/claude-haiku-4-5-20251001 make run`.

## Layout

```
cmd/api            wiring, startup recovery, graceful shutdown (`api migrate` = migrate only)
db/migrations      goose SQL (embedded)       db/queries   sqlc queries
internal/config    env config                 internal/store  sqlc generated
internal/http      router + middleware (auth, CORS, logging, recover), handlers/
internal/ws        hub, client, envelope protocol, per-session broadcast
internal/auth      JWT + bcrypt               internal/dto    contract JSON types
internal/lifecycle per-session cancellable contexts
internal/session   create (channels, DMs, greeting), expiry timer, end -> evaluation
internal/chat      message persistence/broadcast, @mention parsing
internal/persona   persona catalog (prompts/*.yaml, go:embed), reply engine (streaming)
internal/scenario  surprise task scheduler per active session
internal/deadline  response deadlines: DB is source of truth, in-memory timers, startup recovery
internal/validator heuristic + LLM check of replies
internal/evaluation async scoring with the eval model, strict JSON
internal/llm       Provider interface + OpenAI-compatible SSE client, JSON extraction
```

## Behaviour notes

- **Who replies:** in a DM, that persona. In a channel: the @mentioned personas (up to 2), else the author of the thread
  parent, else the chat model picks one (or none — usually none in #watercooler), with a keyword heuristic as fallback.
  Replies stream over WS as `typing` → `message.new` (streaming) → `message.delta`… → `message.done`.
- **Deadlines:** created for every surprise task and for persona messages that @mention the user with a question
  (or that the chat model classifies as a request). Only one auto-created deadline per channel at a time.
  A user message in that channel (top level, or in the request's thread) is checked by the validator; if valid, the deadline is
  RESOLVED (and the task ACKNOWLEDGED). If the reply is rejected, a SYSTEM hint is posted in the thread. On expiry: Warning row,
  session `warnings`+1, SYSTEM message, `deadline.updated`, `warning.new`, `session.updated`.
- **Recovery on startup:** ACTIVE sessions get their expiry timer and task scheduler back (`next_task_at` is persisted);
  expired ones are completed and evaluated; PENDING deadlines are rescheduled (past-due ones expire right away); PENDING
  evaluations are re-run.
- LLM failures never crash the server: persona replies fall back to an "offline" note, the validator falls back to the
  heuristic, task generation falls back to canned tasks, and the evaluation goes to `FAILED`.

## Smoke test

```bash
RESPONSE_TIMEOUT=30s TASK_FIRST_DELAY=20s TASK_MIN_INTERVAL=10m TASK_MAX_INTERVAL=10m make run
make smoke   # in another shell
```

The smoke test registers a user, starts a session, subscribes over WS, DMs Sarah and checks the streamed reply, waits for a
surprise task and a deadline expiry warning, ends the session and polls the evaluation until it is DONE.
