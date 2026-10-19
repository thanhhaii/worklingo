-- name: CreateDeadline :one
INSERT INTO deadlines (session_id, channel_id, message_id, task_id, deadline_at)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetDeadline :one
SELECT * FROM deadlines WHERE id = $1;

-- name: ListDeadlines :many
SELECT * FROM deadlines WHERE session_id = $1 ORDER BY created_at;

-- name: ListPendingDeadlines :many
SELECT d.* FROM deadlines d JOIN sessions s ON s.id = d.session_id
WHERE d.status = 'PENDING' AND s.status = 'ACTIVE';

-- name: ListPendingDeadlinesForChannel :many
SELECT * FROM deadlines WHERE channel_id = $1 AND status = 'PENDING' ORDER BY created_at;

-- name: ResolveDeadline :one
UPDATE deadlines SET status = 'RESOLVED', resolved_at = now()
WHERE id = $1 AND status = 'PENDING' RETURNING *;

-- name: ExpireDeadline :one
UPDATE deadlines SET status = 'EXPIRED', resolved_at = now()
WHERE id = $1 AND status = 'PENDING' RETURNING *;

-- name: CountDeadlines :one
SELECT
  count(*) FILTER (WHERE status = 'RESOLVED')::int AS resolved,
  count(*) FILTER (WHERE status = 'EXPIRED')::int AS expired,
  count(*)::int AS total
FROM deadlines WHERE session_id = $1;
