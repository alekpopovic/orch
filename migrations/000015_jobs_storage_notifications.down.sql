DROP TABLE IF EXISTS notification_sinks;
DROP INDEX IF EXISTS volume_attachments_active_writer;
DROP TABLE IF EXISTS volume_attachments;
DROP TABLE IF EXISTS volume_claims;
DROP TABLE IF EXISTS volumes;
DROP TABLE IF EXISTS cron_jobs;
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_exactly_one_owner;
ALTER TABLE tasks DROP COLUMN IF EXISTS job_id;
ALTER TABLE tasks ALTER COLUMN service_id SET NOT NULL;
DROP TABLE IF EXISTS jobs;
ALTER TABLE services
    DROP COLUMN IF EXISTS gitops_desired_spec,
    DROP COLUMN IF EXISTS gitops_drift_status,
    DROP COLUMN IF EXISTS gitops_source_path,
    DROP COLUMN IF EXISTS gitops_source_commit,
    DROP COLUMN IF EXISTS gitops_source_id;
ALTER TABLE gitops_sources DROP COLUMN IF EXISTS drift_policy;
