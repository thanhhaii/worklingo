# WorkLingo: configurable models and focused writing practice

## Intent and authorization
The user requested implementation of switching between existing model configuration and Gemini, then sequential delivery of the proposed writing-learning features, using subagents to keep the controller context clean. No audio. Prioritize minimal model cost. This document records implementation defaults chosen by the controller; it is not a claim that the user selected each detail.

## Approaches
1. Environment-selected providers with independent chat/evaluation roles (selected by controller): fits existing deployment and avoids credential-management UI.
2. Browser runtime model switching: adds secret storage and authorization complexity without improving writing practice; deferred.
3. Local model hosting: retain compatibility through the existing configurable endpoint; do not introduce hosting dependencies.

## Delivery order
1. Backward-compatible provider profiles and independently selected chat/evaluation providers; Gemini through its OpenAI-compatible endpoint. No automatic paid fallback, no network calls during tests, no secret changes.
2. Honest partial evaluation state and safe evaluation retry, plus model-call usage visibility. These protect learning feedback and free-tier usage.
3. Focused sessions with optional 10/20/60-minute duration and a learning goal, keeping existing requests and configured default duration working. Existing chat workspace remains available.
4. Persisted rewrite practice based on message-review issues, batching up to five answers per judging call, accepting equivalent answers, and retaining drafts on provider failures. Subsequent spaced repetition and progress dashboards remain a later iteration.

## Constraints
Keep Go, PostgreSQL, Next.js, existing authentication and mock mode. Never expose provider credentials in HTTP responses or browser bundles. Provider selection occurs at backend startup; no mid-request or automatic provider switch. Preserve all legacy LLM environment names and defaults. Gemini model IDs remain configurable, with gemini-3.1-flash-lite as the initial profile default. No promise of free quota or correction quality without live benchmarking. Deterministic fixtures test wiring and failure behavior; real model benchmarking requires credentials and is not run implicitly.

## Verification
Use focused behavior tests first, backend full tests and vet, frontend lint and build for changed frontend flows. Cover provider/config precedence, mixed routing, malformed/empty/truncated responses, missing reviews, authorization, input limits, persistence and failed requests. Report missing external verification explicitly.
