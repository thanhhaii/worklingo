-- name: CreateTask :one
INSERT INTO tasks (session_id, channel_id, message_id, persona_id, title, description, priority)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: ListTasks :many
SELECT * FROM tasks WHERE session_id = $1 ORDER BY created_at;

-- name: SetTaskStatus :one
UPDATE tasks SET status = $2 WHERE id = $1 RETURNING *;
