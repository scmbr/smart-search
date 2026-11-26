CREATE TABLE catalog_snapshots (
    id text PRIMARY KEY,
    source_sha256 text NOT NULL,
    schema_version text NOT NULL,
    status text NOT NULL CHECK (status IN ('staging', 'ready', 'invalid')),
    checkpoint bigint NOT NULL DEFAULT 0 CHECK (checkpoint >= 0),
    validation jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (source_sha256, schema_version)
);

CREATE TABLE catalog_records (
    snapshot_id text NOT NULL REFERENCES catalog_snapshots(id),
    ordinal bigint NOT NULL CHECK (ordinal > 0),
    kind text NOT NULL CHECK (kind IN ('category', 'offer')),
    source_id text NOT NULL CHECK (source_id <> ''),
    payload jsonb NOT NULL,
    PRIMARY KEY (snapshot_id, ordinal),
    UNIQUE (snapshot_id, kind, source_id)
);

CREATE TABLE catalog_active (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    snapshot_id text NOT NULL REFERENCES catalog_snapshots(id),
    activated_at timestamptz NOT NULL DEFAULT now()
);
