# WorkLingo backend (AI-Solo)

Go API + WebSocket server for the WorkLingo Slack-like Business English simulator.
The API/WS contract is in [`../docs/CONTRACT.md`](../docs/CONTRACT.md).

Stack: chi, coder/websocket, pgx/v5 + sqlc, goose (embedded migrations, auto-applied on startup),
a small OpenAI-compatible streaming client (SSE), slog, caarlos0/env + godotenv, golang-jwt, bcrypt.

## Run

```bash
export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
docker compose -f ../docker-compose.yml up -d   # Postgres on localhost:5433
cp .env.example .env                            # set JWT_SECRET and the selected provider's API key
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
| `LLM_PROVIDER` | `openai`; accepts `openai` or `gemini` |
| `LLM_CHAT_PROVIDER` / `LLM_EVAL_PROVIDER` | unset; each overrides `LLM_PROVIDER` for its role |
| `LLM_BASE_URL` | `http://127.0.0.1:20128/v1` (any OpenAI-compatible `/chat/completions`) |
| `LLM_API_KEY` | – (sent as `Authorization: Bearer …`) |
| `LLM_CHAT_MODEL` | `cc/claude-haiku-4-5-20251001` — persona replies, responder choice, reply validation, task generation |
| `LLM_EVAL_MODEL` | `cc/claude-sonnet-5-5` — post-session evaluation |
| `LLM_TIMEOUT` | `120s` (HTTP timeout per LLM call) |
| `GEMINI_BASE_URL` | `https://generativelanguage.googleapis.com/v1beta/openai` |
| `GEMINI_API_KEY` | required when either role selects `gemini`; backend only |
| `GEMINI_CHAT_MODEL` / `GEMINI_EVAL_MODEL` | `gemini-3.1-flash-lite` for each role |
| `SESSION_DURATION` | `60m` |
| `RESPONSE_TIMEOUT` | `5m` (deadline length) |
| `TASK_FIRST_DELAY` | `2m` |
| `TASK_MIN_INTERVAL` / `TASK_MAX_INTERVAL` | `6m` / `12m` |

Durations use Go syntax (`30s`, `5m`, `1h`).

### Selecting providers and models

Selection happens at backend startup. Provider names ignore case and surrounding whitespace. Empty role selectors inherit
`LLM_PROVIDER`. Changing a provider, endpoint, key, or model requires restarting the backend; there is no browser selector.
Chat covers persona replies, reply validation, responder selection, tasks, scenarios, and work-context generation.
Evaluation uses its own client and model. `LLM_TIMEOUT` applies to both clients.

Legacy-only configuration is unchanged; selectors may be omitted:

```dotenv
LLM_PROVIDER=openai
LLM_BASE_URL=http://127.0.0.1:20128/v1
LLM_API_KEY=your-legacy-key
LLM_CHAT_MODEL=cc/claude-haiku-4-5-20251001
LLM_EVAL_MODEL=cc/claude-sonnet-5-5
```

Gemini-only configuration uses independent settings. Legacy endpoint, credentials, and models are unused:

```dotenv
LLM_PROVIDER=gemini
GEMINI_API_KEY=your-gemini-key
GEMINI_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
GEMINI_CHAT_MODEL=gemini-3.1-flash-lite
GEMINI_EVAL_MODEL=gemini-3.1-flash-lite
```

For Gemini chat and legacy evaluation, configure both sets of credentials:

```dotenv
LLM_PROVIDER=openai
LLM_CHAT_PROVIDER=gemini
LLM_EVAL_PROVIDER=openai
GEMINI_API_KEY=your-gemini-key
LLM_BASE_URL=https://your-compatible-endpoint.example/v1
LLM_API_KEY=your-legacy-key
LLM_EVAL_MODEL=your-evaluation-model
```

Set `LLM_CHAT_PROVIDER=openai` and `LLM_EVAL_PROVIDER=gemini` for the reverse split. Unsupported providers and missing
selected Gemini keys fail configuration validation before database setup. Empty legacy keys remain allowed for local proxies.
No failure automatically switches to another paid provider.

Gemini uses the existing HTTP client through the [official OpenAI compatibility endpoint](https://ai.google.dev/gemini-api/docs/openai),
with `reasoning_effort=minimal` only on Gemini requests. Existing calls have small output budgets (including 30–50 token
classifications and 400 token chat replies); thinking can consume that budget. Empty or truncated results produce errors,
and streams retain partial text with their error. Choose a suitable model through the provider-specific model variables
if these limits are unsuitable. Provider errors include status codes without logging response bodies or prompts.
Keep all API keys in backend configuration, never in frontend or `NEXT_PUBLIC_*` variables.

For production, set these variables in `.env.prod` and recreate the backend to apply changes:

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build backend
```

Production Compose passes both profiles through without requiring unused legacy credentials. It retains the existing
production legacy model defaults (`claude-haiku-4-5-20251001` and `claude-sonnet-4-6`).

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
  evaluations resume unavailable components and retain persisted successes. An in-process dispatch guard prevents startup recovery from duplicating a newly started evaluation.
- LLM failures never crash the server: persona replies fall back to an "offline" note, the validator falls back to the
  heuristic, and task generation falls back to canned tasks. Evaluation scores and reviews save independently: unavailable scoring produces `FAILED`, while available scores with missing review coverage produce `PARTIAL`. Available scores/reviews stay visible on the report. Missing reviews never mean “No issues”.

## Evaluation retries and usage logs

The session owner can explicitly retry missing feedback with `POST /api/sessions/{id}/evaluation/retry`. It returns `202` with the saved evaluation marked `PENDING`, or `409` for active sessions, running/completed evaluations, or evaluations that have not started. A conditional database update claims failed/partial work before dispatch so concurrent requests cannot start duplicate retries. Successful scores and individual message/task reviews are preserved and skipped. Pending work resumes on restart; the deployment assumes one API process for recovery (multiple replicas would need database worker leases).

Migration `00003_evaluation_integrity.sql` adds component availability and `PARTIAL`. Legacy scores remain available, but historical reviews cannot prove coverage and are marked unavailable until explicitly retried. The existing automatic second scoring attempt was removed to avoid unrequested paid calls.

Every OpenAI-compatible call emits one structured `llm.call` event at INFO with `provider`, `model`, `stream`, `status`, `duration_ms`, `prompt_tokens`, `completion_tokens`, and `total_tokens`. Status describes the upstream call/response, not the later application-level JSON validation. Streaming requests set `stream_options.include_usage`; usage-only SSE events and plain JSON streaming fallbacks are captured. Missing usage fields are `null`, never guessed or converted to a measured dollar cost. Logs exclude prompts, response bodies, credentials and endpoint URLs. Usage availability depends on the configured provider/gateway.

For scratch database integration tests, explicitly set the allowlisted disposable URL:

```bash
TEST_DATABASE_URL='postgres://worklingo:worklingo@localhost:5433/worklingo_codex_20261029?sslmode=disable' go test ./internal/http
```

These integration tests migrate only that database, create their own fixture users (or an isolated schema for upgrade/rollback), and clean them up. Without `TEST_DATABASE_URL`, database tests skip. Other URLs are rejected. Frontend mock retry coverage runs with `node --test evaluation-retry.test.mjs` from `frontend/` and uses the installed TypeScript compiler plus Node's test runner.

## Smoke test

```bash
RESPONSE_TIMEOUT=30s TASK_FIRST_DELAY=20s TASK_MIN_INTERVAL=10m TASK_MAX_INTERVAL=10m make run
make smoke   # in another shell
```

The smoke test registers a user, starts a session, subscribes over WS, DMs Sarah and checks the streamed reply, waits for a
surprise task and a deadline expiry warning, ends the session and polls the evaluation until it is DONE.

## Focused writing sessions

`POST /api/sessions` accepts optional `durationMinutes` (10, 20 or 60) and `learningGoal` (`STATUS_UPDATE`, `CLARIFY_REQUIREMENTS`, `REQUEST_EXTENSION`, `POLITE_PUSHBACK`, `INCIDENT_UPDATE`). Omitted duration preserves `SESSION_DURATION` exactly; omitted goal preserves simulation. Focused setup/task injection is static, without model calls, with 2 tasks for durations <=10m and 3 otherwise. Learner turns use one coverage judge and one responder, without deadlines or ghosting warnings. Explicit session-end evaluation remains a separate model cost and retains partial feedback/retry semantics.

Against an isolated test server, `/usr/local/go/bin/go run ./scripts/smoke -focused` checks focused metadata, static tasks and the absence of deadlines without posting learner turns. Use `-duration-minutes=10` to override the configured duration. The default smoke command keeps its no-payload simulation lifecycle. Automated integration tests only accept `TEST_DATABASE_URL=postgres://worklingo:worklingo@localhost:5433/worklingo_codex_20261029?sslmode=disable`.
