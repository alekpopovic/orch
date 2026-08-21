DROP TABLE IF EXISTS usage_snapshots;
DROP TABLE IF EXISTS maintenance_windows;
ALTER TABLE nodes DROP COLUMN IF EXISTS agent_version;
