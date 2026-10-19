# WorkLingo

**WorkLingo** is a Slack-style workplace simulator for practicing **Business English**. You join a 60-minute "work shift" with 3 AI teammates, answer messages, handle surprise tasks with deadlines, and get an evaluation of your English when the session ends.

> Current mode: **AI-Solo** (1 user + 3 AI personas). This is a personal-use project.

## Features

- **Slack-like workspace**: `#general`, `#sprint-planning`, `#incident-room` and `#watercooler` channels, direct messages, threads, `@mentions`, and Markdown with code highlighting.
- **3 AI teammates**, each with their own personality:

  | Persona | Handle | Role |
  |---|---|---|
  | Sarah Chen | `@pm_sarah` | Product Manager |
  | Alex Novak | `@tech_lead_alex` | Tech Lead |
  | David Miller | `@client_david` | Client (UK) |

- **Real-time streamed replies** over WebSocket (`typing` → `message.delta` → `message.done`).
- **Surprise tasks and deadlines**: the server generates unexpected tasks. If you don't reply in time, you get a **warning** for ghosting.
- **Reply validation**: your replies are checked (heuristic + LLM) to see whether they actually address the request. If they don't, a hint is posted in the thread.
- **End-of-session evaluation**: an evaluation model scores your English communication and gives feedback on a report page.
- **Restart recovery**: active sessions, pending deadlines and unfinished evaluations are restored when the server restarts.
- **Frontend mock mode**: runs entirely in the browser, no backend needed.

## Tech stack

| Part | Technology |
|---|---|
| Backend | Go, chi, coder/websocket, pgx/v5 + sqlc, goose (embedded migrations, applied on startup), slog, JWT + bcrypt |
| Frontend | Next.js (App Router, TypeScript), Tailwind CSS v4, shadcn/ui (Base UI), zustand, react-markdown |
| Database | PostgreSQL 17 (Docker, port `5433`) |
| LLM | Any **OpenAI-compatible** `/chat/completions` endpoint (SSE streaming). Chat and evaluation models are configured separately |

## Project layout

```
.
├── backend/            Go API + WebSocket server (see backend/README.md)
├── frontend/           Next.js app (see frontend/README.md)
├── docs/
│   ├── CONTRACT.md     FE/BE API and WebSocket contract (source of truth)
│   └── architecture.html  Architecture diagram
├── scripts/
│   ├── start.sh        Starts the whole dev stack with one command
│   └── dev-proxy.mjs   Single-port gateway: /api + /ws -> backend, everything else -> Next.js
└── docker-compose.yml  PostgreSQL
```

## Requirements

- Go 1.27+
- Node.js 22 + pnpm
- Docker (for PostgreSQL)
- An API key for an OpenAI-compatible LLM provider

## Quick start

```bash
git clone https://github.com/thanhhaii/worklingo.git
cd worklingo

# 1. Configure the backend
cp backend/.env.example backend/.env
#    -> set LLM_BASE_URL, LLM_API_KEY, LLM_CHAT_MODEL, LLM_EVAL_MODEL, JWT_SECRET

# 2. (optional) Configure the frontend
cp frontend/.env.example frontend/.env.local

# 3. Start everything: Postgres -> backend -> frontend -> proxy
./scripts/start.sh
```

Open **http://localhost:5323**, register an account and start a session. Press `Ctrl+C` to stop. The Postgres container keeps running; use `docker compose down` to stop it. Logs are written to `.logs/`.

Ports can be changed with environment variables: `BACKEND_PORT` (8080), `FRONTEND_PORT` (3000), `PROXY_PORT` (5323).

### Running each part separately

```bash
# Database
docker compose up -d postgres

# Backend (http://localhost:8080)
cd backend && make run

# Frontend (http://localhost:3000), in another shell
cd frontend && pnpm install && pnpm dev
```

### Frontend only (mock mode)

```bash
cd frontend
pnpm install
NEXT_PUBLIC_MOCK=1 pnpm dev
```

## Main configuration (backend/.env)

| Variable | Purpose | Default |
|---|---|---|
| `DATABASE_URL` | Postgres connection string | `postgres://worklingo:worklingo@localhost:5433/worklingo?sslmode=disable` |
| `JWT_SECRET` | JWT signing key, **change it** | insecure dev value |
| `LLM_BASE_URL` | OpenAI-compatible endpoint | `http://127.0.0.1:20128/v1` |
| `LLM_API_KEY` | API key (sent as `Bearer`) | – |
| `LLM_CHAT_MODEL` | Model for chat, reply validation and task generation | `cc/claude-haiku-4-5-20251001` |
| `LLM_EVAL_MODEL` | Model for the end-of-session evaluation | `cc/claude-sonnet-5-5` |
| `SESSION_DURATION` | Length of a session | `60m` |
| `RESPONSE_TIMEOUT` | Time allowed to answer a request | `5m` |
| `TASK_FIRST_DELAY` / `TASK_MIN_INTERVAL` / `TASK_MAX_INTERVAL` | Surprise task timing | `2m` / `6m` / `12m` |

Full list: [`backend/README.md`](backend/README.md). Frontend variables: [`frontend/README.md`](frontend/README.md).

## Development and testing

```bash
# Backend
cd backend
make test     # unit tests
make vet
make sqlc     # regenerate internal/store from db/queries/*.sql
make smoke    # end-to-end test against a running server (see backend/README.md)

# Frontend
cd frontend
pnpm lint
pnpm build    # production build (includes type-check)
```

## Documentation

- [`docs/CONTRACT.md`](docs/CONTRACT.md): data types, REST endpoints, WebSocket protocol, game rules
- [`docs/architecture.html`](docs/architecture.html): architecture diagram
- [`backend/README.md`](backend/README.md): backend details, how the responding persona is chosen, deadlines, recovery
- [`frontend/README.md`](frontend/README.md): frontend details, mock mode

## Security note

Never commit `backend/.env` or `frontend/.env.local` (both are in `.gitignore`). Only the `*.env.example` files belong in the repo.
