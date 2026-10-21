# Spec: Work context, Talking points, per-message review

Status: agreed with the user (grilling session 2026-10-20). Vocabulary: see `/GLOSSARY.md`
(Task, Talking point, Work context, Deadline, Warning, Persona). Use these exact terms in code
and comments (`TalkingPoint`, `WorkContext`), never "mission", "checklist item", "scenario".

## Goal

Today a Task is vague: only a short title is shown (`description` is generated but never
rendered). The learner should instead see **exactly what content they must get across**.

## Decisions (all final)

1. Each session generates ONE **Work context** (fictional project, the feature the learner is
   building, progress, blocker, a few extra facts). Written in **Vietnamese** (proper names may
   stay English). All Talking points draw facts from it, so facts stay consistent in a session.
2. Work context is shown to the learner (right sidebar block "Your work context") and injected
   into **all persona system prompts**, so personas talk about the same feature/progress/blocker.
3. Each Task has **1–3 Talking points** (NORMAL: 1–2, CRITICAL: 2–3), written in **Vietnamese**,
   content only (never English phrasing to copy). A Talking point may be information
   ("Đang làm feature quản lý user, xong khoảng 70%") or a communicative act
   ("Xin lỗi vì trễ hạn", "Đề xuất dời buổi demo sang thứ Năm").
4. The Task `description` field is removed (DB column dropped, DTO field removed).
5. Talking points apply **only to Tasks**. Persona @mention requests without a task keep the
   current deadline/validator behaviour unchanged.
6. Each Talking point is judged individually: `PENDING` → `COVERED`. Covered is sticky.
   Points may be covered across several replies (same channel, or the task's thread).
   - Paraphrase with the right meaning → covered.
   - Facts contradicting the Work context (says 50% when context says 70%) → not covered, with reason.
   - Grammar mistakes do NOT matter for coverage (grammar is graded in the report).
   - A reply not in English → not covered.
7. Task status:
   - `OPEN` → `ACKNOWLEDGED` when its Deadline is resolved (existing validator, unchanged) and
     some points are still pending. Must never downgrade a `DONE` task.
   - → `DONE` as soon as ALL points are covered (regardless of deadline state).
8. **Follow-up**: when the Task's deadline is resolved (task ACKNOWLEDGED) but points are still
   pending after judging a reply, the Task's persona replies **in the task's thread** asking
   specifically (in character, in English, without listing them like a checklist) for the
   missing content, and a **new Deadline linked to the task** is created on that follow-up
   message. **At most 1 follow-up per task.** After that, leftovers go to the report.
9. UI during play: task card in right sidebar and the task chip above the persona message show
   the Talking points with ○ pending / ✓ covered, updated live via `task.updated`. The chip's
   list is collapsible.
10. Report:
    - **Per-task section**: each Task, each Talking point covered/missed, the learner's quote
      that covered it (or best attempt), a reason (Vietnamese) and an **English sample
      sentence** for every point (generated at evaluation time, NOT at task creation).
    - **Message review table** replaces the old "Corrections" list: one row per learner
      message — column 1 the message text, column 2 its issues. ALL issues (no top-10 cap),
      each with category `GRAMMAR|TONE|VOCABULARY` and severity `MAJOR|MINOR`, explanation in
      **Vietnamese**, corrected text in English. Messages with no issues show ✓.
      Columns stack on mobile.

## Contract (source of truth for both FE and BE)

```ts
type WorkContext = {
  project: string;         // e.g. "HR Portal"
  projectSummary: string;  // vi, 1 sentence
  feature: string;         // vi, the feature the learner is building
  progress: string;        // vi, e.g. "Xong khoảng 70%: API và UI danh sách user đã xong, còn phân quyền"
  blocker: string;         // vi
  notes: string[];         // vi, 2-4 extra facts (deadline demo, teammate on leave, tech stack, ...)
};

type Session = { ...existing fields; workContext: WorkContext | null };

type TalkingPoint = {
  id: string;
  position: number;              // 0-based order
  content: string;               // vi
  status: "PENDING" | "COVERED";
  reason: string;                // vi; latest judge reason ("" if never judged)
};

type Task = {
  id; sessionId; channelId; messageId; personaId; title; priority; status; createdAt; // unchanged
  // description: REMOVED
  talkingPoints: TalkingPoint[]; // ordered by position
};

type IssueCategory = "GRAMMAR" | "TONE" | "VOCABULARY";
type IssueSeverity = "MAJOR" | "MINOR";
type MessageIssue = {
  original: string;     // exact substring of the learner message
  corrected: string;    // English
  explanation: string;  // vi
  category: IssueCategory;
  severity: IssueSeverity;
};
type MessageReview = {
  messageId: string;
  channelLabel: string; // "#general" or "DM: Sarah Chen"
  content: string;      // learner message text
  createdAt: string;
  issues: MessageIssue[]; // [] = no issues (✓)
};

type TalkingPointReview = {
  talkingPointId: string;
  content: string;       // vi (copied from the point)
  covered: boolean;      // from DB status, not from the LLM
  learnerQuote: string;  // exact learner text that covered it / best attempt, "" if none
  reason: string;        // vi
  sample: string;        // English sample sentence a professional would write
};
type TaskReview = {
  taskId: string;
  title: string;
  personaId: string;
  status: "OPEN" | "ACKNOWLEDGED" | "DONE";
  talkingPoints: TalkingPointReview[];
};

type Evaluation = {
  id; sessionId; status; grammar; tone; vocabulary; overall; summary; createdAt; // unchanged
  // corrections: REMOVED
  messageReviews: MessageReview[]; // chronological, ALL learner messages
  taskReviews: TaskReview[];       // chronological by task
};
```

REST/WS shapes otherwise unchanged: `task.new` / `task.updated` carry the full `Task` (with
`talkingPoints`); `GET /sessions/{id}` and `POST /sessions` return `session.workContext`.

## Backend design (Go)

- Migration `backend/db/migrations/00002_talking_points.sql` (goose, with Down):
  - `sessions.work_context JSONB` (nullable).
  - `tasks`: drop `description`; add `follow_up_sent BOOLEAN NOT NULL DEFAULT false`.
  - `talking_points` table: `id uuid pk`, `task_id` FK cascade, `position int`, `content text`,
    `status text check PENDING|COVERED default PENDING`, `reason text not null default ''`,
    `covered_message_id uuid null` FK messages set null, `covered_at timestamptz null`.
    Index on `(task_id, position)`.
  - `evaluations`: add `message_reviews JSONB NOT NULL DEFAULT '[]'`, `task_reviews JSONB NOT NULL
    DEFAULT '[]'`. Keep the `corrections` column (old rows) but stop writing/exposing it.
  - Regenerate sqlc (`cd backend && make sqlc`). Never hand-edit `internal/store/*.sql.go`.
- **Work context generation** at session create (`session.Manager.Create`): LLM call (chat
  model, ~30s timeout) BEFORE the DB transaction; on any failure use a static fallback.
  Stored in `sessions.work_context`. Personas are Sarah (PM), Alex (tech lead), David (UK client)
  at a software company; the learner is a software engineer.
- **Persona prompts**: `Catalog.SystemPrompt` gains the work context (rendered as a short block,
  e.g. "Shared work context (the learner's current work; stay consistent with it): ...").
  `Engine.Reply` loads the session to get it. Nil context → no block.
- **Task generation** (`scenario`): prompt includes the work context and asks for
  `"talkingPoints": ["...", ...]` (Vietnamese, 1–3 per the priority rule, consistent with the
  work context, must be answerable in a reply to the persona's message). Remove `description`
  from the prompt/struct. `normalise` trims, drops empties, clamps to 1–3 (if 0, use one
  generic point derived from the title). Fallback tasks get hand-written Vietnamese points.
  Insert task + points; broadcast `task.new` with points.
- **New package `internal/task`** (`task.Service`) owns the per-reply pipeline. The HTTP
  handler for posting a user message calls `Tasks.OnUserMessage(...)` instead of calling
  `Deadlines.OnUserMessage` + `Engine.OnUserMessage` directly. Pipeline (in a goroutine on the
  session ctx; serialise per channel with a mutex):
  1. Find candidate Tasks: same channel, status != DONE, created before the message, having
     PENDING points. If the message is in a thread (`ParentID != nil`), only the task whose
     `MessageID == *ParentID`; if top-level, the (at most 2) most recent such tasks.
  2. If there are no candidates: behave exactly as today (deadline validation and persona
     reply, concurrently). Refactor `deadline.Service` minimally so it exposes a synchronous
     variant (e.g. `ValidateReply(ctx, m) ` that does what the goroutine did) and keep
     `OnUserMessage` for the old path if convenient.
  3. With candidates: run deadline validation and the coverage judge **concurrently**, wait
     for both, then update points (sticky COVERED, store reason + message id), set task
     status DONE if all covered, broadcast `task.updated` (full task with points).
  4. Deadline resolution sets ACKNOWLEDGED only `WHERE status = 'OPEN'` (SQL guard).
  5. Follow-up: for a candidate task with status ACKNOWLEDGED, pending points remaining, and
     `follow_up_sent = false` → atomically set it (`UPDATE ... WHERE follow_up_sent = false
     RETURNING`). Then the persona reply for this message is the follow-up: the engine replies
     as the task persona in the task thread (parent = task.MessageID) with an extra instruction
     to ask for the missing content; afterwards create a Deadline with `task_id` on that reply
     message (skip the engine's own `maybeCreateDeadline` for this reply). Otherwise call the
     normal `Engine.OnUserMessage`. Never produce two persona replies for the same user message
     from the same persona.
  - Coverage judge prompt (chat model, temperature 0, JSON): inputs = work context, task title,
    persona request message, pending points with indexes, learner reply. Output
    `{"points":[{"index":0,"covered":true,"reason":"<vi>"}]}`. Encode rules from Decision 6.
    On LLM failure: leave points unchanged (log warn).
- **Evaluation** (`internal/evaluation`): keep the scoring call (scores + summary) but remove
  corrections from its prompt/result. Add, run concurrently with it:
  - Message review: number the learner messages (`m1..mN`) with a little preceding context;
    batch ≤ 40 messages per call; output `{"messages":[{"id":"m3","issues":[...]}]}`; ALL
    issues, severity MAJOR/MINOR, explanation in Vietnamese; `original` must be an exact
    substring (drop issues whose original isn't found in the message). Every learner message
    becomes a `MessageReview` (no issues → `[]`).
  - Task review: one call over all tasks: for each task give title, persona request, points
    with DB status, and the learner messages in that channel/thread after the task; output per
    point `learnerQuote`, `reason` (vi), `sample` (en). `covered` comes from the DB.
  - Review-call failures are soft: log and fall back (messages with `issues: []`, task review
    with empty quote/reason/sample) — only the scoring call failing fails the evaluation.
  - Use the eval model; generous MaxTokens.
- Update `docs/CONTRACT.md` and `backend/scripts/smoke` if they reference removed fields.
- Unit tests for pure logic: talking point normalisation, candidate task selection, issue
  filtering (exact substring), review assembly.

## Frontend design (Next.js)

- `src/lib/types.ts`: match the contract exactly (remove `description`, `corrections`,
  `Correction`, `CorrectionCategory` if unused).
- `right-sidebar.tsx`: "Your work context" block at the top (project + summary, feature,
  progress, blocker, notes); keep it compact (collapsible is fine). Task cards list their
  Talking points with ○ / ✓ (green check for covered), keep existing card behaviour.
- `message-item.tsx` task chip: collapsible Talking points list (expanded by default while the
  task is not DONE).
- Report page `src/app/session/[id]/report/page.tsx`:
  - New "Tasks" section: per task card (title, persona, status), each point: ✓/✗, Vietnamese
    content, learner quote (if any), reason, and "Sample:" English sentence.
  - Replace "Corrections" with "Message review": table, col 1 = message (channel label + time
    + text), col 2 = issues (strike-through original → bold corrected, category badge,
    MAJOR/MINOR marker, Vietnamese explanation) or a ✓ "No issues". Stack on mobile.
    Show counts (messages, issues, major issues).
- Mock mode (`src/lib/mock/server.ts`): generate a static work context, tasks with points,
  naive coverage (e.g. each valid reply covers the next pending point), one follow-up per task,
  and a mock evaluation with `messageReviews` + `taskReviews` so the UI is fully exercisable
  without backend.
- Keep the existing visual style (Slack-like, Tailwind classes, shadcn components in
  `src/components/ui`). Run `pnpm exec tsc --noEmit`, `pnpm lint`, `pnpm build`.
