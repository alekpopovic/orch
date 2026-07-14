ALTER TABLE services
    ADD COLUMN security_context JSONB NOT NULL DEFAULT '{}'::jsonb;
