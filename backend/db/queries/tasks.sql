-- name: CreateTask :one
INSERT INTO tasks (session_id, channel_id, message_id, persona_id, title, priority)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasks :many
SELECT * FROM tasks WHERE session_id = $1 ORDER BY created_at;

-- name: SetTaskStatus :one
UPDATE tasks SET status = $2 WHERE id = $1 RETURNING *;

-- name: AcknowledgeTask :one
UPDATE tasks SET status = 'ACKNOWLEDGED' WHERE id = $1 AND status = 'OPEN' RETURNING *;

-- name: MarkTaskDone :one
UPDATE tasks SET status = 'DONE' WHERE id = $1 AND status <> 'DONE' RETURNING *;

-- name: ClaimTaskFollowUp :one
UPDATE tasks SET follow_up_sent = true WHERE id = $1 AND follow_up_sent = false RETURNING *;
