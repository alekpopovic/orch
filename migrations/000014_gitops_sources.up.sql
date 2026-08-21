CREATE TABLE gitops_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    namespace TEXT NOT NULL REFERENCES namespaces (name) ON DELETE CASCADE,
    repository_url TEXT NOT NULL,
    branch TEXT NOT NULL,
    path TEXT NOT NULL,
    sync_interval_ns BIGINT NOT NULL CHECK (sync_interval_ns > 0),
    prune BOOLEAN NOT NULL DEFAULT false,
    managed_services JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_revision TEXT,
    last_error TEXT,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);

CREATE INDEX idx_gitops_sources_namespace ON gitops_sources (namespace, created_at);
