CREATE TABLE search_sessions (
    session_id text NOT NULL,
    idempotency_key text NOT NULL,
    body_sha256 text NOT NULL CHECK (body_sha256 ~ '^[a-f0-9]{64}$'),
    search_id text NOT NULL UNIQUE,
    result jsonb NOT NULL CHECK (jsonb_typeof(result)='object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '24 hours',
    PRIMARY KEY(session_id,idempotency_key),
    CHECK (expires_at >= created_at + interval '24 hours')
);
