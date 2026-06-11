CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hostname TEXT NOT NULL,
    advertise_address TEXT NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}'::jsonb,
    capacity_cpu BIGINT NOT NULL CHECK (capacity_cpu >= 0),
    capacity_memory BIGINT NOT NULL CHECK (capacity_memory >= 0),
    allocatable_cpu BIGINT NOT NULL CHECK (allocatable_cpu >= 0),
    allocatable_memory BIGINT NOT NULL CHECK (allocatable_memory >= 0),
    status TEXT NOT NULL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    version BIGINT NOT NULL DEFAULT 1,
    CHECK (allocatable_cpu <= capacity_cpu),
    CHECK (allocatable_memory <= capacity_memory)
);

CREATE INDEX idx_nodes_status ON nodes (status);

CREATE TABLE services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    image TEXT NOT NULL,
    replicas INTEGER NOT NULL CHECK (replicas >= 0),
    env JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    ports JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
    healthcheck JSONB,
    restart_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    placement_constraints JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'active',
    deployment_version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    version BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE service_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    version BIGINT NOT NULL,
    spec JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    UNIQUE (service_id, version)
);

CREATE INDEX idx_service_versions_service_id ON service_versions (service_id);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    node_id UUID REFERENCES nodes (id) ON DELETE SET NULL,
    container_id TEXT,
    desired_status TEXT NOT NULL,
    actual_status TEXT NOT NULL,
    image TEXT NOT NULL,
    version BIGINT NOT NULL,
    restart_count INTEGER NOT NULL DEFAULT 0 CHECK (restart_count >= 0),
    failure_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    row_version BIGINT NOT NULL DEFAULT 1
);

CREATE INDEX idx_tasks_service_id ON tasks (service_id);
CREATE INDEX idx_tasks_node_id ON tasks (node_id);
CREATE INDEX idx_tasks_status ON tasks (actual_status, desired_status);

CREATE TABLE deployments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services (id) ON DELETE CASCADE,
    from_version BIGINT NOT NULL,
    to_version BIGINT NOT NULL,
    strategy TEXT NOT NULL,
    status TEXT NOT NULL,
    max_unavailable INTEGER NOT NULL CHECK (max_unavailable >= 0),
    max_surge INTEGER NOT NULL CHECK (max_surge >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now()),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    row_version BIGINT NOT NULL DEFAULT 1
);

CREATE INDEX idx_deployments_service_id ON deployments (service_id);

CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type TEXT NOT NULL,
    severity TEXT NOT NULL,
    source TEXT NOT NULL,
    message TEXT NOT NULL,
    related_object_type TEXT,
    related_object_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);

CREATE INDEX idx_events_related_object_created_at
    ON events (related_object_type, related_object_id, created_at DESC);
CREATE INDEX idx_events_type_created_at ON events (type, created_at DESC);
CREATE INDEX idx_events_severity_created_at ON events (severity, created_at DESC);
