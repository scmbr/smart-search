CREATE TABLE api_budget (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    cap_nano bigint NOT NULL CHECK (cap_nano > 0 AND cap_nano <= 5000000000)
);
INSERT INTO api_budget(singleton,cap_nano) VALUES(true,5000000000);
CREATE TABLE api_reservations (
    id text PRIMARY KEY,
    model text NOT NULL,
    dimensions integer NOT NULL,
    upper_nano bigint NOT NULL CHECK (upper_nano > 0),
    actual_nano bigint CHECK (actual_nano > 0 AND actual_nano <= upper_nano),
    input_tokens bigint CHECK (input_tokens > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    settled_at timestamptz,
    CHECK ((actual_nano IS NULL) = (settled_at IS NULL))
);
CREATE TABLE embedding_cache (
    cache_key text PRIMARY KEY CHECK (length(cache_key)=64),
    vector jsonb NOT NULL CHECK (jsonb_typeof(vector)='array'),
    created_at timestamptz NOT NULL DEFAULT now()
);
