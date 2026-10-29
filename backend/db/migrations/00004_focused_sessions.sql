-- +goose Up
ALTER TABLE sessions ADD COLUMN learning_goal TEXT CHECK (learning_goal IN ('STATUS_UPDATE','CLARIFY_REQUIREMENTS','REQUEST_EXTENSION','POLITE_PUSHBACK','INCIDENT_UPDATE')),
ADD COLUMN duration_minutes INTEGER CHECK (duration_minutes IN (10,20,60)),
ADD COLUMN focused_step INTEGER NOT NULL DEFAULT 0 CHECK (focused_step BETWEEN 0 AND 3);
-- +goose Down
ALTER TABLE sessions DROP COLUMN focused_step, DROP COLUMN duration_minutes, DROP COLUMN learning_goal;
