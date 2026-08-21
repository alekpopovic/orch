DELETE FROM audit_logs WHERE namespace <> 'default';
DELETE FROM events WHERE namespace <> 'default';
DELETE FROM deployments WHERE namespace <> 'default';
DELETE FROM tasks WHERE namespace <> 'default';
DELETE FROM service_versions WHERE service_id IN (SELECT id FROM services WHERE namespace <> 'default');
DELETE FROM services WHERE namespace <> 'default';
DELETE FROM secrets WHERE namespace <> 'default';
DELETE FROM registry_credentials WHERE namespace <> 'default';

DROP INDEX IF EXISTS idx_audit_logs_namespace_created_at;
DROP INDEX IF EXISTS idx_events_namespace_created_at;
DROP INDEX IF EXISTS idx_deployments_namespace;
DROP INDEX IF EXISTS idx_tasks_namespace;
DROP INDEX IF EXISTS idx_services_namespace;

ALTER TABLE registry_credentials DROP CONSTRAINT registry_credentials_pkey;
ALTER TABLE registry_credentials ADD PRIMARY KEY (id);
ALTER TABLE secrets DROP CONSTRAINT secrets_pkey;
ALTER TABLE secrets ADD PRIMARY KEY (name);
ALTER TABLE services DROP CONSTRAINT services_namespace_name_key;
ALTER TABLE services ADD UNIQUE (name);

ALTER TABLE audit_logs DROP COLUMN namespace;
ALTER TABLE registry_credentials DROP COLUMN namespace;
ALTER TABLE secrets DROP COLUMN namespace;
ALTER TABLE events DROP COLUMN namespace;
ALTER TABLE deployments DROP COLUMN namespace;
ALTER TABLE tasks DROP COLUMN namespace;
ALTER TABLE services DROP COLUMN namespace;

DROP TABLE namespaces;
