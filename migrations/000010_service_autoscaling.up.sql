ALTER TABLE services
    ADD COLUMN autoscaling JSONB NOT NULL DEFAULT '{}'::jsonb;
