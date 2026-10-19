-- name: CreateWarning :one
INSERT INTO warnings (session_id, deadline_id, reason) VALUES ($1, $2, $3) RETURNING *;

-- name: ListWarnings :many
SELECT * FROM warnings WHERE session_id = $1 ORDER BY created_at;
