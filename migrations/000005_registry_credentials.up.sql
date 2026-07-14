ALTER TABLE services
    ADD COLUMN image_pull_secret TEXT NOT NULL DEFAULT '';

CREATE TABLE registry_credentials (
    id TEXT PRIMARY KEY,
    registry TEXT NOT NULL,
    username TEXT NOT NULL,
    encrypted_password BYTEA NOT NULL,
    key_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);
