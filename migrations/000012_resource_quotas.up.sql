CREATE TABLE resource_quotas (
    namespace TEXT PRIMARY KEY REFERENCES namespaces (name) ON DELETE CASCADE,
    max_services INTEGER NOT NULL DEFAULT 0 CHECK (max_services >= 0),
    max_replicas INTEGER NOT NULL DEFAULT 0 CHECK (max_replicas >= 0),
    max_cpu_millicores BIGINT NOT NULL DEFAULT 0 CHECK (max_cpu_millicores >= 0),
    max_memory_bytes BIGINT NOT NULL DEFAULT 0 CHECK (max_memory_bytes >= 0),
    max_public_ports INTEGER NOT NULL DEFAULT 0 CHECK (max_public_ports >= 0),
    max_secrets INTEGER NOT NULL DEFAULT 0 CHECK (max_secrets >= 0),
    max_registry_credentials INTEGER NOT NULL DEFAULT 0 CHECK (max_registry_credentials >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT timezone('utc', now())
);

INSERT INTO resource_quotas (namespace)
SELECT name FROM namespaces;
