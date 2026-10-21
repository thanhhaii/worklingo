-- +goose Up
ALTER TABLE sessions ADD COLUMN work_context JSONB;

ALTER TABLE tasks DROP COLUMN description;
ALTER TABLE tasks ADD COLUMN follow_up_sent BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE talking_points (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id            UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    position           INT NOT NULL,
    content            TEXT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','COVERED')),
    reason             TEXT NOT NULL DEFAULT '',
    covered_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    covered_at         TIMESTAMPTZ
);
CREATE INDEX talking_points_task_idx ON talking_points(task_id, position);

ALTER TABLE evaluations ADD COLUMN message_reviews JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE evaluations ADD COLUMN task_reviews JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE evaluations DROP COLUMN task_reviews;
ALTER TABLE evaluations DROP COLUMN message_reviews;
DROP TABLE talking_points;
ALTER TABLE tasks DROP COLUMN follow_up_sent;
ALTER TABLE tasks ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions DROP COLUMN work_context;
