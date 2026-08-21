ALTER TABLE gitops_sources ADD COLUMN drift_policy TEXT NOT NULL DEFAULT 'warn' CHECK (drift_policy IN ('warn','auto_revert'));

ALTER TABLE services
    ADD COLUMN gitops_source_id UUID REFERENCES gitops_sources(id) ON DELETE SET NULL,
    ADD COLUMN gitops_source_commit TEXT,
    ADD COLUMN gitops_source_path TEXT,
    ADD COLUMN gitops_drift_status TEXT,
    ADD COLUMN gitops_desired_spec JSONB;

CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    namespace TEXT NOT NULL REFERENCES namespaces(name),
    name TEXT NOT NULL,
    spec JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending','running','succeeded','failed')),
    attempts INTEGER NOT NULL DEFAULT 1,
    last_exit_code INTEGER,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    UNIQUE(namespace, name, created_at)
);

ALTER TABLE tasks
    ALTER COLUMN service_id DROP NOT NULL,
    ADD COLUMN job_id UUID REFERENCES jobs(id) ON DELETE CASCADE,
    ADD CONSTRAINT tasks_exactly_one_owner CHECK ((service_id IS NOT NULL) <> (job_id IS NOT NULL));

CREATE TABLE cron_jobs (
    id UUID PRIMARY KEY,
    namespace TEXT NOT NULL REFERENCES namespaces(name),
    name TEXT NOT NULL,
    spec JSONB NOT NULL,
    last_schedule_at TIMESTAMPTZ,
    next_schedule_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE(namespace, name)
);

CREATE TABLE volumes (
    id UUID PRIMARY KEY,
    namespace TEXT NOT NULL REFERENCES namespaces(name),
    name TEXT NOT NULL,
    driver TEXT NOT NULL CHECK (driver = 'local'),
    node_id UUID REFERENCES nodes(id),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(namespace, name)
);

CREATE TABLE volume_claims (
    id UUID PRIMARY KEY,
    namespace TEXT NOT NULL REFERENCES namespaces(name),
    name TEXT NOT NULL,
    volume_id UUID REFERENCES volumes(id),
    access_mode TEXT NOT NULL CHECK (access_mode IN ('ReadWriteOnce','ReadWriteMany')),
    allow_concurrent_writers BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(namespace, name)
);

CREATE TABLE volume_attachments (
    id UUID PRIMARY KEY,
    volume_id UUID NOT NULL REFERENCES volumes(id),
    task_id UUID NOT NULL REFERENCES tasks(id),
    node_id UUID NOT NULL REFERENCES nodes(id),
    read_only BOOLEAN NOT NULL DEFAULT FALSE,
    attached_at TIMESTAMPTZ NOT NULL,
    detached_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX volume_attachments_active_writer
    ON volume_attachments(volume_id) WHERE detached_at IS NULL AND read_only = FALSE;

CREATE TABLE notification_sinks (
    id UUID PRIMARY KEY,
    namespace TEXT NOT NULL REFERENCES namespaces(name),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('webhook','slack','http')),
    url TEXT NOT NULL,
    encrypted_signing_secret BYTEA,
    key_id TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(namespace, name)
);
