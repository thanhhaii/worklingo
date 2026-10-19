# WorkLingo frontend (AI-Solo)

A Slack-style simulator where you practice Business English with 3 AI teammates.
Next.js (App Router, TypeScript), Tailwind CSS v4, shadcn/ui (Base UI), zustand, react-markdown + remark-gfm + rehype-highlight.

The binding API contract is [`../docs/CONTRACT.md`](../docs/CONTRACT.md).

## Run

Requires Node 22 and pnpm.

```bash
cd frontend
cp .env.example .env.local      # adjust if needed
pnpm install
pnpm dev                        # http://localhost:3000
```

### Environment

| Variable | Default | Purpose |
|---|---|---|
| `NEXT_PUBLIC_API_URL` | `http://localhost:8080/api` | Go REST API base |
| `NEXT_PUBLIC_WS_URL` | `ws://localhost:8080/ws` | WebSocket endpoint (`?token=<jwt>` appended) |
| `NEXT_PUBLIC_MOCK` | `0` | `1` = in-browser fake API + fake WebSocket, no backend needed |

`NEXT_PUBLIC_*` values are inlined at build time, so restart `pnpm dev` (or rebuild) after changing them.

### Mock mode (no backend)

```bash
NEXT_PUBLIC_MOCK=1 pnpm dev
```

Register any account (stored in `localStorage`), start a session, and the fake server will:

- stream a greeting from Sarah in #general (~2s) and a code-heavy message from Alex in #sprint-planning (~6s)
- drop a **CRITICAL task** in #incident-room at ~15s, with a 5-minute deadline
- DM you from David at ~25s with a **75-second** deadline (shortened so you can watch it expire into a ghosting warning plus a SYSTEM message)
- reply (streamed) to anything you post: DMs go to that persona; in channels, `@handle` picks the persona, otherwise the channel's default one
- run a fake evaluation ~4s after you end the session

Reset mock data with `localStorage.removeItem("worklingo.mockdb.v1")`. Simulate a dropped socket from the devtools console with `__mockDropWs()` to watch reconnect + gap refetch.

## Scripts

- `pnpm dev`: dev server
- `pnpm build`: production build (type-checks)
- `pnpm start`: serve the production build
- `pnpm lint`: ESLint

## Structure

```
src/app/                       routes: / (dashboard), /login, /register, /session/[id], /session/[id]/report
src/components/workspace/      Slack UI: sidebar, message list/item, composer, thread pane, right sidebar
src/hooks/use-session-controller.ts   data loading + WebSocket wiring + toasts
src/store/workspace.ts         zustand store; applies every server WS event
src/store/auth.ts              JWT auth (token in localStorage)
src/lib/api.ts                 REST client (swappable transport, mock or fetch)
src/lib/ws.ts                  WS client: subscribe, 25s ping, backoff reconnect, re-subscribe, onReconnect refetch
src/lib/mock/server.ts         in-browser fake backend (REST + WS) for NEXT_PUBLIC_MOCK=1
```

## Notes

- The app is entirely client-rendered against the Go API. The JWT lives in `localStorage` (`worklingo.token`), and a 401 clears it and redirects to `/login`.
- Deadline countdowns are display-only and computed from the server's `deadlineAt`. The server decides when a deadline is resolved or expired.
- Messages are deduped by `id`. Optimistic user messages are swapped for the server copy when the POST returns.
