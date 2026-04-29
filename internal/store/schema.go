package store

const PostgresSchema = `
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    user_id VARCHAR(64) UNIQUE NOT NULL,
    username VARCHAR(64) UNIQUE NOT NULL,
    nickname VARCHAR(128) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    phone VARCHAR(64) NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    role VARCHAR(32) NOT NULL DEFAULT 'user',
    last_login_at TIMESTAMPTZ NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(128) UNIQUE NOT NULL,
    event_name VARCHAR(256) NOT NULL DEFAULT '',
    title VARCHAR(256) NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    context TEXT NOT NULL DEFAULT '',
    source VARCHAR(64) NOT NULL DEFAULT '',
    severity VARCHAR(32) NOT NULL DEFAULT 'medium',
    category VARCHAR(128) NOT NULL DEFAULT '',
    event_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    current_round INTEGER NOT NULL DEFAULT 1,
    observables JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    message_id VARCHAR(128) UNIQUE NOT NULL,
    event_id VARCHAR(128) NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    user_id VARCHAR(128) NOT NULL DEFAULT '',
    user_nickname VARCHAR(128) NOT NULL DEFAULT '',
    message_from VARCHAR(64) NOT NULL DEFAULT '',
    message_type VARCHAR(64) NOT NULL DEFAULT '',
    message_content TEXT NOT NULL DEFAULT '',
    round_id INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tasks (
    id BIGSERIAL PRIMARY KEY,
    task_id VARCHAR(128) UNIQUE NOT NULL,
    event_id VARCHAR(128) NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    task_name VARCHAR(256) NOT NULL DEFAULT '',
    task_description TEXT NOT NULL DEFAULT '',
    task_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    task_priority VARCHAR(32) NOT NULL DEFAULT '',
    assigned_to VARCHAR(128) NOT NULL DEFAULT '',
    round_id INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS executions (
    id BIGSERIAL PRIMARY KEY,
    execution_id VARCHAR(128) UNIQUE NOT NULL,
    event_id VARCHAR(128) NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    command_id VARCHAR(128) NOT NULL DEFAULT '',
    execution_status VARCHAR(32) NOT NULL DEFAULT 'pending',
    execution_result TEXT NOT NULL DEFAULT '',
    command_name VARCHAR(256) NOT NULL DEFAULT '',
    command_type VARCHAR(64) NOT NULL DEFAULT '',
    command_entity VARCHAR(256) NOT NULL DEFAULT '',
    command_params TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS summaries (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(128) NOT NULL REFERENCES events(event_id) ON DELETE CASCADE,
    round_id INTEGER NOT NULL DEFAULT 1,
    event_summary TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS event_maps (
    fingerprint VARCHAR(128) PRIMARY KEY,
    ly_event_id VARCHAR(128) NOT NULL DEFAULT '',
    deepsoc_event_id VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sync_cursors (
    name VARCHAR(64) PRIMARY KEY,
    last_ts VARCHAR(128) NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pushed_events (
    ly_event_id VARCHAR(128) PRIMARY KEY,
    idempotency_key VARCHAR(128) UNIQUE NOT NULL,
    deepsoc_event_id VARCHAR(128) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'SUCCESS',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor VARCHAR(128) NOT NULL DEFAULT 'unknown',
    action VARCHAR(64) NOT NULL DEFAULT '',
    target VARCHAR(256) NOT NULL DEFAULT '',
    meta TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS prompts (
    role VARCHAR(64) PRIMARY KEY,
    content TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS settings (
    key VARCHAR(128) PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_event_id ON messages(event_id, id);
CREATE INDEX IF NOT EXISTS idx_tasks_event_id ON tasks(event_id);
CREATE INDEX IF NOT EXISTS idx_executions_event_id ON executions(event_id);
CREATE INDEX IF NOT EXISTS idx_summaries_event_id ON summaries(event_id);
CREATE INDEX IF NOT EXISTS idx_pushed_events_idempotency_key ON pushed_events(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);
`
