-- name: MaterializePracticeItem :exec
INSERT INTO practice_items(user_id,session_id,evaluation_id,message_id,issue_index,snapshot)
SELECT $1,$2,$3,$4,$5,$6 WHERE EXISTS(SELECT 1 FROM sessions WHERE id=$2 AND user_id=$1)
ON CONFLICT(evaluation_id,message_id,issue_index) DO NOTHING;

-- name: ListPracticeItems :many
SELECT * FROM practice_items WHERE session_id=$1 AND user_id=$2 ORDER BY created_at,id;

-- name: ClaimPracticeBatch :one
INSERT INTO practice_batches(request_id,user_id,session_id,answers)
SELECT $1,$2,$3,$4 WHERE EXISTS(SELECT 1 FROM sessions WHERE id=$3 AND user_id=$2)
ON CONFLICT(request_id) DO NOTHING RETURNING *;

-- name: GetPracticeBatch :one
SELECT * FROM practice_batches WHERE request_id=$1 AND user_id=$2 AND session_id=$3;

-- name: FinishPracticeBatch :one
UPDATE practice_batches SET status=$4,results=$5,completed_at=now()
WHERE request_id=$1 AND user_id=$2 AND session_id=$3 AND status='PENDING' RETURNING *;

-- name: ListPracticeBatches :many
SELECT * FROM practice_batches WHERE session_id=$1 AND user_id=$2 ORDER BY created_at,request_id;

-- name: RecoverPracticeBatches :exec
UPDATE practice_batches SET status='UNAVAILABLE',completed_at=now(),
results=COALESCE((SELECT jsonb_agg(answer || jsonb_build_object(
 'verdict','UNAVAILABLE','feedback',sqlc.arg(feedback)::text,'attemptedAt',created_at))
 FROM jsonb_array_elements(answers) answer),'[]'::jsonb)
WHERE status='PENDING';
