-- +goose Up
CREATE TABLE practice_items (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 evaluation_id UUID NOT NULL REFERENCES evaluations(id) ON DELETE CASCADE,
 message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
 issue_index INTEGER NOT NULL,
 snapshot JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(evaluation_id,message_id,issue_index)
);
CREATE INDEX practice_items_session ON practice_items(session_id,user_id);
-- Answers are checkpointed at claim time; results form immutable attempt history.
CREATE TABLE practice_batches (
 request_id UUID PRIMARY KEY,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 answers JSONB NOT NULL,
 results JSONB NOT NULL DEFAULT '[]',
 status TEXT NOT NULL DEFAULT 'PENDING' CHECK(status IN ('PENDING','COMPLETE','UNAVAILABLE')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_at TIMESTAMPTZ
);
CREATE INDEX practice_batches_session ON practice_batches(session_id,user_id,created_at);
-- +goose Down
DROP TABLE practice_batches;
DROP TABLE practice_items;
