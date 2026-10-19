-- +goose Up
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode         TEXT NOT NULL DEFAULT 'AI_SOLO',
    status       TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','COMPLETED','ABORTED')),
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    ended_at     TIMESTAMPTZ,
    warnings     INT NOT NULL DEFAULT 0,
    next_task_at TIMESTAMPTZ
);
CREATE INDEX sessions_user_idx ON sessions(user_id, started_at DESC);
CREATE UNIQUE INDEX sessions_one_active_per_user ON sessions(user_id) WHERE status = 'ACTIVE';

CREATE TABLE channels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('CHANNEL','DM')),
    name       TEXT NOT NULL,
    persona_id TEXT,
    position   INT NOT NULL DEFAULT 0,
    UNIQUE (session_id, kind, name)
);

CREATE TABLE messages (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    parent_id   UUID REFERENCES messages(id) ON DELETE SET NULL,
    author_type TEXT NOT NULL CHECK (author_type IN ('USER','PERSONA','SYSTEM')),
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    persona_id  TEXT,
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX messages_channel_idx ON messages(channel_id, created_at);
CREATE INDEX messages_session_idx ON messages(session_id, created_at);

CREATE TABLE tasks (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id  UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    persona_id  TEXT NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    priority    TEXT NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('NORMAL','CRITICAL')),
    status      TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN','ACKNOWLEDGED','DONE')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tasks_session_idx ON tasks(session_id, created_at);

CREATE TABLE deadlines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    channel_id  UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    message_id  UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    task_id     UUID REFERENCES tasks(id) ON DELETE SET NULL,
    status      TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','RESOLVED','EXPIRED')),
    deadline_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);
CREATE INDEX deadlines_session_idx ON deadlines(session_id, created_at);
CREATE INDEX deadlines_pending_idx ON deadlines(status) WHERE status = 'PENDING';

CREATE TABLE warnings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    deadline_id UUID NOT NULL REFERENCES deadlines(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE evaluations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL UNIQUE REFERENCES sessions(id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','DONE','FAILED')),
    grammar     INT NOT NULL DEFAULT 0,
    tone        INT NOT NULL DEFAULT 0,
    vocabulary  INT NOT NULL DEFAULT 0,
    overall     INT NOT NULL DEFAULT 0,
    summary     TEXT NOT NULL DEFAULT '',
    corrections JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE evaluations;
DROP TABLE warnings;
DROP TABLE deadlines;
DROP TABLE tasks;
DROP TABLE messages;
DROP TABLE channels;
DROP TABLE sessions;
DROP TABLE users;
