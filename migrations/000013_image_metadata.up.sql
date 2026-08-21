ALTER TABLE services ADD COLUMN image_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE tasks ADD COLUMN requested_image TEXT;
ALTER TABLE tasks ADD COLUMN resolved_image_digest TEXT;
ALTER TABLE tasks ADD COLUMN image_registry TEXT;
ALTER TABLE tasks ADD COLUMN image_name TEXT;
ALTER TABLE tasks ADD COLUMN image_tag TEXT;

UPDATE tasks SET requested_image = image WHERE requested_image IS NULL;
