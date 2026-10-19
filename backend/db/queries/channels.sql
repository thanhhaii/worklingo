-- name: CreateChannel :one
INSERT INTO channels (session_id, kind, name, persona_id, position) VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListChannels :many
SELECT * FROM channels WHERE session_id = $1 ORDER BY position;

-- name: GetChannel :one
SELECT * FROM channels WHERE id = $1;
