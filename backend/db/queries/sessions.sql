-- name: CreateSession :one
INSERT INTO sessions (user_id, expires_at, next_task_at, work_context) VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: GetActiveSessionForUser :one
SELECT * FROM sessions WHERE user_id = $1 AND status = 'ACTIVE' LIMIT 1;

-- name: ListSessionsForUser :many
SELECT * FROM sessions WHERE user_id = $1 ORDER BY started_at DESC;

-- name: ListActiveSessions :many
SELECT * FROM sessions WHERE status = 'ACTIVE';

-- name: CompleteSession :one
UPDATE sessions SET status = 'COMPLETED', ended_at = now()
WHERE id = $1 AND status = 'ACTIVE' RETURNING *;

-- name: IncrementSessionWarnings :one
UPDATE sessions SET warnings = warnings + 1 WHERE id = $1 RETURNING *;

-- name: SetSessionNextTaskAt :exec
UPDATE sessions SET next_task_at = $2 WHERE id = $1;
