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

type LearningGoal = "STATUS_UPDATE" | "CLARIFY_REQUIREMENTS" | "REQUEST_EXTENSION" | "POLITE_PUSHBACK" | "INCIDENT_UPDATE";
type CreateSessionOptions = { durationMinutes?: 10 | 20 | 60; learningGoal?: LearningGoal };

type Session = {
  id: string; mode: "AI_SOLO"; status: "ACTIVE" | "COMPLETED" | "ABORTED";
  startedAt: string; expiresAt: string; endedAt: string | null;
  learningGoal: LearningGoal | null; // null = full simulation
  durationMinutes: 10 | 20 | 60 | null; // null = configured SESSION_DURATION used
  warnings: number;            // ghosting warnings count
  workContext: WorkContext | null; // deterministic in focused sessions, generated in simulation (null for old sessions)
}

type WorkContext = {           // fictional project situation; Vietnamese (proper names may stay English)
  project: string;             // e.g. "HR Portal"
  projectSummary: string;      // 1 sentence
  feature: string;             // the feature the learner is building
  progress: string;            // e.g. "Xong khoảng 70%: API và UI danh sách user đã xong, còn phân quyền"
  blocker: string;
  notes: string[];             // 2-4 extra facts
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
  personaId: string; title: string;
  priority: "NORMAL" | "CRITICAL";
  status: "OPEN" | "ACKNOWLEDGED" | "DONE";  // ACKNOWLEDGED = deadline resolved or focused attempt judged; DONE = all talking points covered
  createdAt: string;
  talkingPoints: TalkingPoint[];             // ordered by position, 1-3
}

type TalkingPoint = {   // content the learner must get across (Vietnamese, content only)
  id: string;
  position: number;     // 0-based
  content: string;
  status: "PENDING" | "COVERED";  // COVERED is sticky
  reason: string;       // Vietnamese; latest judge reason ("" if never judged)
}

type Deadline = {   // 5-minute response requirement, server-authoritative
  id: string; sessionId: string; channelId: string; messageId: string; // message requiring response
  taskId: string | null;
  status: "PENDING" | "RESOLVED" | "EXPIRED";
  deadlineAt: string; createdAt: string; resolvedAt: string | null;
}

type Warning = { id: string; sessionId: string; deadlineId: string; reason: string; createdAt: string }

type IssueCategory = "GRAMMAR" | "TONE" | "VOCABULARY"
type IssueSeverity = "MAJOR" | "MINOR"
type MessageIssue = {
  original: string;     // exact substring of the learner message
  corrected: string;    // English
  explanation: string;  // Vietnamese
  category: IssueCategory; severity: IssueSeverity;
}
type ReviewStatus = "AVAILABLE" | "UNAVAILABLE"
type MessageReview = {  // one per learner message
  reviewStatus: ReviewStatus;
  messageId: string;
  channelLabel: string; // "#general" or "DM: Sarah Chen"
  content: string; createdAt: string;
  issues: MessageIssue[]; // [] means no issues ONLY when reviewStatus is AVAILABLE
}

type TalkingPointReview = {
  talkingPointId: string;
  content: string;       // Vietnamese
  covered: boolean;      // from DB status
  learnerQuote: string;  // exact learner text that covered it / best attempt, "" if none
  reason: string;        // Vietnamese
  sample: string;        // English sample sentence ("" if the review call failed)
}
type TaskReview = {
  reviewStatus: ReviewStatus; // detailed feedback coverage; independent of task status
  taskId: string; title: string; personaId: string;
  status: "OPEN" | "ACKNOWLEDGED" | "DONE";
  talkingPoints: TalkingPointReview[];
}

type Evaluation = {
  id: string; sessionId: string;
  status: "PENDING" | "DONE" | "PARTIAL" | "FAILED";
  scoreStatus: ReviewStatus;
  messageReviewStatus: ReviewStatus;
  taskReviewStatus: ReviewStatus;
  grammar: number; tone: number; vocabulary: number; overall: number;  // 0-100
  summary: string; // usable score feedback only when scoreStatus is AVAILABLE
  messageReviews: MessageReview[]; // chronological, ALL learner messages; always an array
  taskReviews: TaskReview[];       // chronological by task; always an array
  createdAt: string;
}
```

Evaluation availability:

- `PENDING` means work is running. Previously saved scores/reviews remain available during a retry.
- `DONE` means scores and all requested message/task reviews are available. `PARTIAL` means scores are available but some reviews are unavailable. `FAILED` means scoring is unavailable; independently successful reviews are still returned.
- Each message/task has `reviewStatus`. Missing IDs, missing/null issues, malformed or duplicate responses, incomplete task points, unusable corrections/quotes, and failed batches are `UNAVAILABLE`. An explicit `issues: []` on an available message means it was checked without issues.
- Aggregate review status is `AVAILABLE` only when every corresponding entry is available, including a known empty set. An empty array with unavailable coverage does not prove the session had no messages/tasks. Task `status` and point `covered` still come from session progress and are independent of detailed review availability.
- Legacy completed reports retain their scores but become `PARTIAL`: previous review arrays lack reliable coverage, so entries without an explicit review status are unavailable until retried.
- Retry atomically claims failed/partial work before dispatch, preserving successful score, message and task components. Only unavailable messages/tasks are requested again. Authentication and session ownership match GET; another user's session returns 404. Retries are explicit; no automatic paid scoring retry or provider fallback occurs.

## REST
| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/health` | – | `{ "ok": true }` |
| POST | `/auth/register` | `{username,email,password}` | `{ token, user }` |
| POST | `/auth/login` | `{email,password}` | `{ token, user }` |
| GET | `/me` | – | `User` |
| GET | `/personas` | – | `Persona[]` |
| POST | `/sessions` | `CreateSessionOptions` (or `{}` / empty body) | `Session` (creates 4 channels + 3 DMs; focused static task or simulation greeting). 400 for invalid/null/type-mismatched options. 409 if user already has ACTIVE session. |
| GET | `/sessions/active` | – | `Session` or 404 |
| GET | `/sessions` | – | `Session[]` (history, newest first) |
| GET | `/sessions/{id}` | – | `{ session, channels: Channel[], tasks: Task[], deadlines: Deadline[], warnings: Warning[] }` |
| POST | `/sessions/{id}/end` | – | `Session` (status COMPLETED, triggers evaluation) |
| GET | `/sessions/{id}/channels/{channelId}/messages?before=<iso>&limit=50` | – | `Message[]` ascending by createdAt |
| POST | `/sessions/{id}/channels/{channelId}/messages` | `{content, parentId?}` | `Message` (also broadcast via WS) |
| GET | `/sessions/{id}/evaluation` | – | `Evaluation` (404 if not started) |
| POST | `/sessions/{id}/evaluation/retry` | – | `202 Evaluation` with status PENDING; owner only, completed/aborted sessions only; `409 evaluation_conflict` if active, running, complete or no evaluation |

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
- Omitted duration uses `SESSION_DURATION` exactly (default 60m, including seconds-long smoke overrides); an explicit `durationMinutes` of 10, 20 or 60 overrides it. `expiresAt` is the countdown authority. On expiry → COMPLETED + evaluation.
- Supplying `learningGoal` selects focused practice; omitting it preserves full simulation, including for an explicit short duration. Invalid goals, null options, non-integer/unsupported durations are 400 errors before provider work. Legacy metadata is exposed as null.
- Focused practice uses deterministic work facts and three prewritten templates per goal, delivering at most two tasks for actual durations <=10m and three otherwise. Initial context and task injection make zero model calls. The first task is due after min(1.5s, duration/20); subsequent tasks use absolute offsets evenly spaced over the session. Task/message/points, private step and next due time commit in one transaction. Recovery never replays committed tasks and spaces overdue catch-up tasks by at least 1.5s; exhausted and expired sessions generate no tasks.
- Focused sessions have no response deadlines or ghosting warnings. Reply in each task’s thread. Exactly one responder is selected: parent persona, first recognized mention, newest channel task persona, DM persona, then goal default. A turn uses at most one coverage judge and one streamed reply/follow-up, without responder classification, request detection, or deadline validation. Focused evaluation sees the goal and facts, keeps the same score shape/weights and retry contracts, and does not penalize response speed or professional pushback/clarification.
- The remaining rules below apply to full simulation:
- Personas reply when the user posts in a channel/DM (DM → that persona; channel → @mentioned persona(s), else the most relevant persona, may be none in #watercooler).
- Surprise tasks: during the session, a persona drops a task in a channel at random intervals (config `TASK_MIN_INTERVAL`/`TASK_MAX_INTERVAL`, default 6–12 min, first one ~2 min after start). Each task creates a Deadline.
- Persona messages that explicitly @mention the user with a question/request also create a Deadline.
- Deadline = 5 min (`RESPONSE_TIMEOUT`). Resolved when the user posts a *valid* English reply in that channel (or thread) addressing it. Expired → Warning + SYSTEM message in channel.
- In AI-Solo, warnings only affect score; no lockout.

## Rewrite practice

Authenticated, session-owned endpoints (paths below are under `/api`):

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/sessions/{id}/practice` | – | `{items: PracticeItem[], sourceReviewUnavailable: boolean, pendingBatches: PracticeSubmission[], completedBatches: PracticeResult[]}` |
| POST | `/sessions/{id}/practice/attempts` | `PracticeSubmission` | `{requestId, attempts: PracticeAttempt[]}` |

`PracticeSubmission = {requestId: UUID, answers: {exerciseId: UUID, answer: string}[]}`. Supply 1–5 distinct exercises from that session. Answers are trimmed, nonempty, and at most 4000 Unicode characters. JSON bodies are limited to 256 KiB; unknown fields and trailing JSON are rejected. Invalid input/unknown exercises return 400; another owner's session returns 404. Rubrics and context always come from the server.

`PracticeItem = {id, messageId, category, channelLabel, original, target, correction, explanation, context: string[], attempts: PracticeAttempt[]}`. `original` is the full source message; `target` is the reviewed issue fragment; `correction` is a sample full-message rewrite. Only `AVAILABLE` message reviews with valid nonempty issue fields seed items. GET idempotently materializes immutable item snapshots with stable UUIDs, including for existing reports, without a model call. Context includes at most two preceding messages from the same channel/thread, each capped at 1000 characters. Missing source review coverage is explicitly signaled and never treated as a clean report. Arrays are always present.

`PracticeAttempt = {exerciseId, answer, verdict: "ACCEPTED"|"NEEDS_WORK"|"UNAVAILABLE", feedback, attemptedAt}`. All submitted answers are judged semantically in one call using the configured evaluation provider/model. Equivalent wording that preserves meaning, fixes the target issue and uses professional tone is accepted. Feedback is Vietnamese. The entire batch becomes `UNAVAILABLE` on network/quota failure or malformed, incomplete, duplicate, unknown-ID or otherwise invalid output. Practice validates the entire response as JSON; a valid-looking first object followed by another value, truncated content or garbage is unavailable. That state is neither a wrong answer nor a score. Submitted answers remain durable, including when the client disconnects.

A durable request UUID claim prevents repeated judging: a completed identical submission replays the saved response; a running duplicate or changed payload with the same ID returns 409. The client retains the ID after an ambiguous network failure. Confirmed unavailable results can be deliberately retried with a new ID; there are no automatic model retries or provider fallbacks. GET returns running claims separately as `pendingBatches`, without inventing terminal feedback, and terminal `{requestId, attempts}` results as `completedBatches` (`PracticeResult[]`). A persisted pending ID is cleared only when that exact ID appears in completedBatches, including after an ambiguous POST response. An absent ID or matching older answer does not prove completion. The page can reload saved feedback without another model call. Single-process startup changes interrupted claims to unavailable, retaining answers. A process loss after upstream success but before the checkpoint can require a deliberate fresh retry and another external call; this flow does not promise exactly-once external execution across crashes.

The report links to `/session/{id}/practice`. The page hides sample corrections until explicit reveal, preserves drafts and pending request IDs per user/session in localStorage, and displays persisted feedback/history on reload. Mock mode stores the same lifecycle locally but uses clearly labeled sample-answer fixtures (including a known alternate wording), not a real semantic judge. Spaced repetition and progress dashboards remain deferred; no audio is included.
