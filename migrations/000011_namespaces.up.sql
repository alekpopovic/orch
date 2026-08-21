CREATE TABLE namespaces (
    name TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    CHECK (name ~ '^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$')
);

INSERT INTO namespaces (name) VALUES ('default');

ALTER TABLE services ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);
ALTER TABLE tasks ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);
ALTER TABLE deployments ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);
ALTER TABLE events ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);
ALTER TABLE secrets ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);
ALTER TABLE registry_credentials ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);
ALTER TABLE audit_logs ADD COLUMN namespace TEXT NOT NULL DEFAULT 'default' REFERENCES namespaces (name);

ALTER TABLE services DROP CONSTRAINT services_name_key;
ALTER TABLE services ADD CONSTRAINT services_namespace_name_key UNIQUE (namespace, name);

ALTER TABLE secrets DROP CONSTRAINT secrets_pkey;
ALTER TABLE secrets ADD PRIMARY KEY (namespace, name);

ALTER TABLE registry_credentials DROP CONSTRAINT registry_credentials_pkey;
ALTER TABLE registry_credentials ADD PRIMARY KEY (namespace, id);

CREATE INDEX idx_services_namespace ON services (namespace, status);
CREATE INDEX idx_tasks_namespace ON tasks (namespace, actual_status, desired_status);
CREATE INDEX idx_deployments_namespace ON deployments (namespace, status);
CREATE INDEX idx_events_namespace_created_at ON events (namespace, created_at DESC);
CREATE INDEX idx_audit_logs_namespace_created_at ON audit_logs (namespace, created_at DESC);
