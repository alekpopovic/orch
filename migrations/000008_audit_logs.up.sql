CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'agent', 'system')),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    request_id TEXT,
    source_ip TEXT,
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);

CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at DESC);
CREATE INDEX idx_audit_logs_actor_created_at ON audit_logs (actor_type, actor_id, created_at DESC);
CREATE INDEX idx_audit_logs_target_created_at ON audit_logs (target_type, target_id, created_at DESC);
CREATE INDEX idx_audit_logs_action_created_at ON audit_logs (action, created_at DESC);
