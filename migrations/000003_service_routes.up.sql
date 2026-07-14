ALTER TABLE services
    ADD COLUMN routes JSONB NOT NULL DEFAULT '[]'::jsonb;
