DROP TABLE IF EXISTS registry_credentials;

ALTER TABLE IF EXISTS services
    DROP COLUMN IF EXISTS image_pull_secret;
