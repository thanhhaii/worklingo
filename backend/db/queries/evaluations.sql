-- name: UpsertPendingEvaluation :one
INSERT INTO evaluations (session_id, status) VALUES ($1, 'PENDING')
ON CONFLICT (session_id) DO NOTHING RETURNING *;

-- name: GetEvaluation :one
SELECT * FROM evaluations WHERE session_id = $1;

-- name: ClaimEvaluationRetry :one
UPDATE evaluations e SET status = 'PENDING'
WHERE e.session_id = $1 AND e.status IN ('FAILED', 'PARTIAL')
AND EXISTS (SELECT 1 FROM sessions s WHERE s.id = e.session_id AND s.status <> 'ACTIVE')
RETURNING e.*;

-- name: SaveEvaluationScores :exec
UPDATE evaluations SET score_status = 'AVAILABLE', grammar = $2, tone = $3, vocabulary = $4, overall = $5, summary = $6
WHERE session_id = $1 AND status = 'PENDING';

-- name: SaveEvaluationMessageReviews :exec
UPDATE evaluations SET message_reviews = $2, message_review_status = $3 WHERE session_id = $1 AND status = 'PENDING';

-- name: SaveEvaluationTaskReviews :exec
UPDATE evaluations SET task_reviews = $2, task_review_status = $3 WHERE session_id = $1 AND status = 'PENDING';

-- name: FinalizeEvaluation :one
UPDATE evaluations SET status = CASE
WHEN score_status <> 'AVAILABLE' THEN 'FAILED'
WHEN message_review_status = 'AVAILABLE' AND task_review_status = 'AVAILABLE' THEN 'DONE'
ELSE 'PARTIAL' END
WHERE session_id = $1 AND status = 'PENDING' RETURNING *;

-- name: ListPendingEvaluations :many
SELECT * FROM evaluations WHERE status = 'PENDING';
