ALTER TABLE events
    ADD COLUMN details JSONB NOT NULL DEFAULT '{}'::jsonb;
