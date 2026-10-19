-- name: UpsertPendingEvaluation :one
INSERT INTO evaluations (session_id, status) VALUES ($1, 'PENDING')
ON CONFLICT (session_id) DO UPDATE SET status = 'PENDING'
RETURNING *;

-- name: GetEvaluation :one
SELECT * FROM evaluations WHERE session_id = $1;

-- name: CompleteEvaluation :one
UPDATE evaluations SET status = 'DONE', grammar = $2, tone = $3, vocabulary = $4, overall = $5, summary = $6, corrections = $7
WHERE session_id = $1 RETURNING *;

-- name: FailEvaluation :one
UPDATE evaluations SET status = 'FAILED', summary = $2 WHERE session_id = $1 RETURNING *;

-- name: ListPendingEvaluations :many
SELECT * FROM evaluations WHERE status = 'PENDING';
