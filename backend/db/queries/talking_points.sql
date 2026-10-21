-- name: CreateTalkingPoint :one
INSERT INTO talking_points (task_id, position, content) VALUES ($1, $2, $3) RETURNING *;

-- name: ListTalkingPointsForTask :many
SELECT * FROM talking_points WHERE task_id = $1 ORDER BY position;

-- name: ListTalkingPointsForSession :many
SELECT tp.* FROM talking_points tp JOIN tasks t ON t.id = tp.task_id
WHERE t.session_id = $1 ORDER BY t.created_at, tp.position;

-- name: MarkTalkingPointCovered :one
UPDATE talking_points SET status = 'COVERED', reason = $2, covered_message_id = $3, covered_at = now()
WHERE id = $1 AND status = 'PENDING' RETURNING *;

-- name: SetTalkingPointReason :exec
UPDATE talking_points SET reason = $2 WHERE id = $1 AND status = 'PENDING';
