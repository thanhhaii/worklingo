-- name: CreateMessage :one
INSERT INTO messages (session_id, channel_id, parent_id, author_type, user_id, persona_id, content)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetMessage :one
SELECT * FROM messages WHERE id = $1;

-- name: UpdateMessageContent :one
UPDATE messages SET content = $2 WHERE id = $1 RETURNING *;

-- name: DeleteMessage :exec
DELETE FROM messages WHERE id = $1;

-- name: ListMessagesBefore :many
SELECT * FROM (
  SELECT * FROM messages
  WHERE channel_id = $1 AND created_at < $2
  ORDER BY created_at DESC
  LIMIT $3
) m ORDER BY created_at ASC;

-- name: ListSessionMessages :many
SELECT * FROM messages WHERE session_id = $1 ORDER BY created_at ASC;
