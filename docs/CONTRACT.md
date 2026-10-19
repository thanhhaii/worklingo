# WorkLingo — FE/BE Contract (AI-Solo MVP)

Single source of truth between `frontend/` (Next.js, port 3000) and `backend/` (Go, port 8080).
Change this file first if the contract must change.

## Conventions
- Base URL: `http://localhost:8080/api`. WebSocket: `ws://localhost:8080/ws?token=<jwt>`.
- JSON, camelCase fields. Timestamps: RFC3339 UTC strings. IDs: UUID strings.
- Auth: `Authorization: Bearer <jwt>` on all endpoints except `/auth/*` and `/health`.
- Errors: HTTP 4xx/5xx with body `{ "error": { "code": "string", "message": "string" } }`.
- CORS allows `http://localhost:3000`.

## Personas (static)
| id | handle | displayName | role | avatarColor |
|---|---|---|---|---|
| `sarah` | `pm_sarah` | Sarah Chen | Product Manager | `#E01E5A` |
| `alex` | `tech_lead_alex` | Alex Novak | Tech Lead | `#2EB67D` |
| `david` | `client_david` | David Miller | Client (UK) | `#ECB22E` |

## Types
```ts
type User = { id: string; username: string; email: string; createdAt: string }

type Persona = { id: string; handle: string; displayName: string; role: string; avatarColor: string }

type Session = {
  id: string; mode: "AI_SOLO"; status: "ACTIVE" | "COMPLETED" | "ABORTED";
  startedAt: string; expiresAt: string; endedAt: string | null;
  warnings: number;            // ghosting warnings count
}

type Channel = {
  id: string; sessionId: string;
  kind: "CHANNEL" | "DM";
  name: string;                // "general" | "sprint-planning" | "incident-room" | "watercooler" for CHANNEL; persona id for DM
  personaId: string | null;    // set for DM
}

type Message = {
  id: string; sessionId: string; channelId: string;
  parentId: string | null;     // thread reply
  authorType: "USER" | "PERSONA" | "SYSTEM";
  userId: string | null; personaId: string | null;
  content: string;             // markdown
  createdAt: string;
  streaming?: boolean;         // true while persona is still generating (only in WS events)
}

type Task = {
  id: string; sessionId: string; channelId: string; messageId: string; // message that announced the task
  personaId: string; title: string; description: string;
  priority: "NORMAL" | "CRITICAL";
  status: "OPEN" | "ACKNOWLEDGED" | "DONE";
  createdAt: string;
}

type Deadline = {   // 5-minute response requirement, server-authoritative
  id: string; sessionId: string; channelId: string; messageId: string; // message requiring response
  taskId: string | null;
  status: "PENDING" | "RESOLVED" | "EXPIRED";
  deadlineAt: string; createdAt: string; resolvedAt: string | null;
}

type Warning = { id: string; sessionId: string; deadlineId: string; reason: string; createdAt: string }

type Evaluation = {
  id: string; sessionId: string;
  status: "PENDING" | "DONE" | "FAILED";
  grammar: number; tone: number; vocabulary: number; overall: number;  // 0-100
  summary: string;
  corrections: { original: string; corrected: string; explanation: string; category: "GRAMMAR" | "TONE" | "VOCABULARY" }[];
  createdAt: string;
}
```

## REST
| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/health` | – | `{ "ok": true }` |
| POST | `/auth/register` | `{username,email,password}` | `{ token, user }` |
| POST | `/auth/login` | `{email,password}` | `{ token, user }` |
| GET | `/me` | – | `User` |
| GET | `/personas` | – | `Persona[]` |
| POST | `/sessions` | `{}` | `Session` (creates 4 channels + 3 DMs, persona greeting kicks off). 409 if user already has ACTIVE session. |
| GET | `/sessions/active` | – | `Session` or 404 |
| GET | `/sessions` | – | `Session[]` (history, newest first) |
| GET | `/sessions/{id}` | – | `{ session, channels: Channel[], tasks: Task[], deadlines: Deadline[], warnings: Warning[] }` |
| POST | `/sessions/{id}/end` | – | `Session` (status COMPLETED, triggers evaluation) |
| GET | `/sessions/{id}/channels/{channelId}/messages?before=<iso>&limit=50` | – | `Message[]` ascending by createdAt |
| POST | `/sessions/{id}/channels/{channelId}/messages` | `{content, parentId?}` | `Message` (also broadcast via WS) |
| GET | `/sessions/{id}/evaluation` | – | `Evaluation` (404 if not started) |

## WebSocket
Envelope both directions: `{ "type": string, "data": object }`.
After connecting, client sends `subscribe` for the session it is viewing.

### Client → Server
| type | data |
|---|---|
| `subscribe` | `{ sessionId }` |
| `typing` | `{ sessionId, channelId }` |
| `ping` | `{}` |

### Server → Client
| type | data |
|---|---|
| `pong` | `{}` |
| `message.new` | `Message` (user, system, or persona message; persona messages start with `streaming:true`, content `""`) |
| `message.delta` | `{ messageId, channelId, delta: string }` (append to content) |
| `message.done` | `Message` (final full content, streaming:false) |
| `typing` | `{ channelId, personaId }` (persona is typing; clear after 4s or on message.new) |
| `task.new` | `Task` |
| `task.updated` | `Task` |
| `deadline.new` | `Deadline` |
| `deadline.updated` | `Deadline` (RESOLVED or EXPIRED) |
| `warning.new` | `{ warning: Warning, totalWarnings: number }` |
| `session.updated` | `Session` (e.g. COMPLETED on expiry or end) |
| `evaluation.updated` | `Evaluation` |
| `error` | `{ code, message }` |

## Game rules (backend-enforced)
- Session lasts 60 minutes (`SESSION_DURATION`, configurable). On expiry → COMPLETED + evaluation.
- Personas reply when the user posts in a channel/DM (DM → that persona; channel → @mentioned persona(s), else the most relevant persona, may be none in #watercooler).
- Surprise tasks: during the session, a persona drops a task in a channel at random intervals (config `TASK_MIN_INTERVAL`/`TASK_MAX_INTERVAL`, default 6–12 min, first one ~2 min after start). Each task creates a Deadline.
- Persona messages that explicitly @mention the user with a question/request also create a Deadline.
- Deadline = 5 min (`RESPONSE_TIMEOUT`). Resolved when the user posts a *valid* English reply in that channel (or thread) addressing it. Expired → Warning + SYSTEM message in channel.
- In AI-Solo, warnings only affect score; no lockout.
