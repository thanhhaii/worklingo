-- +goose Up
ALTER TABLE evaluations DROP CONSTRAINT evaluations_status_check;
ALTER TABLE evaluations ADD CONSTRAINT evaluations_status_check CHECK (status IN ('PENDING','DONE','PARTIAL','FAILED'));
ALTER TABLE evaluations ADD COLUMN score_status TEXT NOT NULL DEFAULT 'UNAVAILABLE' CHECK (score_status IN ('AVAILABLE','UNAVAILABLE'));
ALTER TABLE evaluations ADD COLUMN message_review_status TEXT NOT NULL DEFAULT 'UNAVAILABLE' CHECK (message_review_status IN ('AVAILABLE','UNAVAILABLE'));
ALTER TABLE evaluations ADD COLUMN task_review_status TEXT NOT NULL DEFAULT 'UNAVAILABLE' CHECK (task_review_status IN ('AVAILABLE','UNAVAILABLE'));
-- Old scores are usable, but old review arrays cannot prove review coverage.
UPDATE evaluations SET score_status = 'AVAILABLE', status = 'PARTIAL' WHERE status = 'DONE';

-- +goose Down
UPDATE evaluations SET status = 'DONE' WHERE status = 'PARTIAL';
ALTER TABLE evaluations DROP COLUMN task_review_status;
ALTER TABLE evaluations DROP COLUMN message_review_status;
ALTER TABLE evaluations DROP COLUMN score_status;
ALTER TABLE evaluations DROP CONSTRAINT evaluations_status_check;
ALTER TABLE evaluations ADD CONSTRAINT evaluations_status_check CHECK (status IN ('PENDING','DONE','FAILED'));
