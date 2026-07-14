ALTER TABLE services
    ADD COLUMN stateful BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE tasks
    ADD COLUMN conditions JSONB NOT NULL DEFAULT '[]'::jsonb;
