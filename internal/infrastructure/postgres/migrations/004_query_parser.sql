ALTER TABLE api_reservations ADD COLUMN output_tokens bigint CHECK (output_tokens >= 0);
CREATE TABLE query_parser_cache (
    cache_key text PRIMARY KEY CHECK (cache_key ~ '^[a-f0-9]{64}$'),
    entry jsonb NOT NULL CHECK (jsonb_typeof(entry)='object'),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE query_parser_pending (
    cache_key text PRIMARY KEY CHECK (cache_key ~ '^[a-f0-9]{64}$'),
    reservation_id text NOT NULL UNIQUE REFERENCES api_reservations(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
