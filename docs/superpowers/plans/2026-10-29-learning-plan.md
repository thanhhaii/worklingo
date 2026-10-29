# Configurable models and focused writing implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Switch existing/Gemini model profiles safely, then deliver lower-cost writing feedback and practice.
**Architecture:** Select backend provider configurations independently per role, retain the existing client abstraction, extend existing session/evaluation APIs and authenticated UI rather than adding infrastructure.
**Tech Stack:** Go, chi, pgx/sqlc, PostgreSQL, Next.js, TypeScript, zustand.
**Spec:** docs/superpowers/specs/2026-10-29-learning-design.md

## Global Constraints
- No audio, no new hosting or agent-framework dependencies.
- Preserve legacy LLM environment configuration and empty session creation requests.
- API keys stay backend-only; never log secrets, prompts, or learner message bodies.
- No automatic paid fallback. Provider selection happens at startup.
- Preserve mock mode, existing session history and authentication.
- Read frontend/AGENTS.md and relevant installed Next.js docs before frontend edits.
- Implement and review tasks sequentially, with meaningful behavior tests and no worker-spawned subagents.

## Review Focus
- Mixed provider configuration routes evaluation independently of chat.
- Missing or truncated AI responses never become a successful review.
- A retry cannot overwrite an in-progress evaluation or duplicate billed work.
- Session settings survive reload and are represented consistently in mock mode.
- Rewrite requests cannot reference another user's evaluation or lose answers on upstream failure.

### Task 1: Provider profiles and role switching

**Files:** backend/internal/config/config.go; new config/provider tests; backend/internal/llm/openai.go and tests; backend/cmd/api/main.go; backend/.env.example; .env.prod.example; docker-compose.prod.yml; backend/README.md; README.md.

**Interfaces:** Preserve llm.Provider. Add config.LLMProfile with Provider, BaseURL, APIKey, Model; expose Config.ChatLLM() and Config.EvalLLM() resolving profiles (return profile/error). llm.NewOpenAICompat stays callable for legacy callers. Extend client options through fields or a constructor without duplicating transport.

- [ ] Write failing configuration tests: absent selectors preserves current base URL/key/models; LLM_PROVIDER=gemini chooses Gemini for both roles; LLM_CHAT_PROVIDER/LLM_EVAL_PROVIDER override global independently; unsupported provider returns error; selected Gemini missing key returns actionable config error without secrets; whitespace/case handling; selected Gemini URL/key/models do not accidentally inherit legacy profile values.
- [ ] Implement LLM_PROVIDER (default openai), optional LLM_CHAT_PROVIDER/LLM_EVAL_PROVIDER; retain LLM_BASE_URL, LLM_API_KEY, LLM_CHAT_MODEL, LLM_EVAL_MODEL; add GEMINI_BASE_URL (default https://generativelanguage.googleapis.com/v1beta/openai), GEMINI_API_KEY, GEMINI_CHAT_MODEL/GEMINI_EVAL_MODEL (default gemini-3.1-flash-lite). LLM_TIMEOUT shared. Accept openai and gemini, optionally normalize openai-compatible alias if useful. Validate before database setup.
- [ ] Write httptest integration tests for selected endpoint URL, auth, model, normal completion, SSE, non-stream JSON fallback, HTTP 429, truncated/empty completion and malformed/incomplete streams. Implement robust failure reporting; preserve partial text with error. Bound Gemini thinking using reasoning_effort=minimal for selected Gemini profile without sending that field to legacy endpoints. Gemini Complete JSON requests need no new SDK.
- [ ] Wire separate provider clients to existing chat/validator/task/scenario/work-context services vs evaluation service. Safe startup logs show provider and model only.
- [ ] Update development and production examples, Compose passthrough without requiring unused legacy credentials, and docs with legacy-only/Gemini-only/mixed examples and restart semantics. Never touch actual .env files.
- [ ] Run focused tests, then go test ./... and go vet ./... from backend (PATH includes /usr/local/go/bin). Commit only task files; report RED/GREEN and commands.

### Task 2: Evaluation integrity, retry and usage visibility

**Files:** evaluation service/reviews/tests, DTO/types/report UI, store queries/migration if required, HTTP handlers/router, LLM client instrumentation, mock and contract documentation.

**Interfaces:** Add explicit message/task review status; POST /sessions/{id}/evaluation/retry authorizes session owner, excludes active sessions and atomically claims FAILED/partial work before dispatch. Return conflict for already-running work. Preserve existing score fields and report rendering.

- [ ] Test missing review IDs and failed batches are unavailable, not clean; partial states reach UI. Retry covers failures and partial work and cannot double-dispatch.
- [ ] Implement explicit partial status and retry of failed components, preserving successful components and ownership checks. Keep schemas backward compatible; expose retry in report and mock.
- [ ] Record model-call duration, provider/model, status and usage counts (including streaming usage when available); unknown usage remains unknown. Do not log inputs or credentials, do not estimate dollars as measured cost.
- [ ] Run backend tests/vet, frontend lint/build, update contract and docs, commit and report.

### Task 3: Focused writing sessions

**Files:** session/scenario/persona/evaluation packages, migration and sqlc queries/models, session DTO/handlers, frontend types/api/dashboard/workspace/mock and docs.

**Interfaces:** POST /sessions accepts optional durationMinutes:10|20|60 and learningGoal enum; missing settings preserve server duration and existing simulation. Persist settings and expose via session DTO. Goals: STATUS_UPDATE, CLARIFY_REQUIREMENTS, REQUEST_EXTENSION, POLITE_PUSHBACK, INCIDENT_UPDATE.

- [ ] Test request validation, configured default compatibility, persisted settings, goal-aware prompts and focused static scenario selection.
- [ ] Implement optional focused sessions using prewritten task templates and bounded task count 2 for 10 minutes/3 for 20 minutes; retain current 60-minute simulation. Focused mode selects one responder per turn and avoids an LLM call to generate initial work context/scenarios.
- [ ] Add dashboard duration/goal controls and consistent UI descriptions; reflect goal in workspace/report and mock mode. Existing default simulation remains available.
- [ ] Run tests and frontend checks, update contract/docs, commit and report.

### Task 4: Rewrite practice from reviewed mistakes

**Files:** new backend/internal/practice service/tests, migration/sqlc queries, HTTP handlers/router/wiring/DTO, frontend practice UI/API/types/report entry point/mock, contract/docs.

**Interfaces:** Authenticated GET /sessions/{id}/practice returns exercises from successful message review issues and persisted latest attempts; POST /sessions/{id}/practice/attempts accepts 1-5 {exerciseId,answer} entries. Server owns original/context/correction; client cannot supply its own grading rubric. Structured response includes per-exercise verdict, feedback and attempt timestamp. Errors leave previous attempts intact.

- [ ] Write behavior tests for exercise extraction, ownership, 1-5 batch limit, duplicate/unknown IDs, answer length, accepting alternate wording through semantic judge, malformed/incomplete judge output and quota failure.
- [ ] Implement stable exercise IDs based on source message/issue, persisted attempts, one judging call per submitted batch with minimal necessary context and Vietnamese feedback. Validate every result ID before committing the batch. Exact known corrections may be accepted locally only when faithful to the full sentence; do not string-match all open answers.
- [ ] Add report entry and focused practice page/component that initially hides sample corrections, retains drafts on failure, and shows feedback/history after submission. Preserve mock mode.
- [ ] Run backend tests/vet and frontend lint/build; document deferred spaced repetition/dashboard, commit and report.
