ALTER TABLE api_reservations ADD COLUMN outcome text NOT NULL DEFAULT 'unresolved'
    CHECK (outcome IN ('unresolved', 'rejected', 'settled'));
UPDATE api_reservations SET outcome='settled' WHERE actual_nano IS NOT NULL;
CREATE TABLE embedding_pending (
    cache_key text PRIMARY KEY CHECK (cache_key ~ '^[a-f0-9]{64}$'),
    reservation_id text NOT NULL REFERENCES api_reservations(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX embedding_pending_reservation ON embedding_pending(reservation_id);
