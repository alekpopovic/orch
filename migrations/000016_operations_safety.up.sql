CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);

ALTER TABLE nodes ADD COLUMN IF NOT EXISTS agent_version TEXT;

CREATE TABLE IF NOT EXISTS maintenance_windows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    namespace TEXT REFERENCES namespaces(name) ON DELETE CASCADE,
    global_scope BOOLEAN NOT NULL DEFAULT FALSE,
    schedule TEXT NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'UTC',
    duration_ns BIGINT NOT NULL CHECK (duration_ns > 0),
    allowed_operations JSONB NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    CHECK (global_scope OR namespace IS NOT NULL),
    UNIQUE NULLS NOT DISTINCT (namespace, name)
);

CREATE TABLE IF NOT EXISTS usage_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    namespace TEXT NOT NULL REFERENCES namespaces(name) ON DELETE CASCADE,
    captured_at TIMESTAMPTZ NOT NULL,
    cpu_millicores BIGINT NOT NULL,
    memory_bytes BIGINT NOT NULL,
    replicas INTEGER NOT NULL,
    services INTEGER NOT NULL,
    task_runtime_seconds DOUBLE PRECISION NOT NULL,
    public_ports INTEGER NOT NULL,
    storage_claims INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS usage_snapshots_namespace_time ON usage_snapshots(namespace, captured_at);
