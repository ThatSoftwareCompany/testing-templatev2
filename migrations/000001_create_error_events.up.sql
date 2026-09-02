CREATE TABLE IF NOT EXISTS error_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL,
    correlation_id TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    status_code INTEGER NOT NULL CHECK (status_code >= 500 AND status_code <= 599),
    error_code TEXT NOT NULL,
    message TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS error_events_endpoint_occurred_at_idx
    ON error_events (endpoint, occurred_at DESC);
