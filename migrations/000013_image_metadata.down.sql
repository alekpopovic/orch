ALTER TABLE tasks DROP COLUMN image_tag;
ALTER TABLE tasks DROP COLUMN image_name;
ALTER TABLE tasks DROP COLUMN image_registry;
ALTER TABLE tasks DROP COLUMN resolved_image_digest;
ALTER TABLE tasks DROP COLUMN requested_image;
ALTER TABLE services DROP COLUMN image_metadata;
