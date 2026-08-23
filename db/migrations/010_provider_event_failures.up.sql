BEGIN;

CREATE TABLE provider_event_failures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    provider TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    event_id TEXT NOT NULL,

    payment_id UUID REFERENCES payments(id),
    provider_ref TEXT,
    merchant_id TEXT NOT NULL,

    amount_currency CHAR(3) NOT NULL,
    amount_minor BIGINT NOT NULL,
    customer_display TEXT,

    kind TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,

    status TEXT NOT NULL DEFAULT 'retryable',
    attempts INTEGER NOT NULL DEFAULT 1,

    last_error TEXT NOT NULL,

    first_failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_failed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,

    CONSTRAINT provider_event_failures_attempts_positive
        CHECK (attempts > 0),

    CONSTRAINT provider_event_failures_status_valid
        CHECK (
            status IN (
                'retryable',
                'dead_letter',
                'resolved'
            )
        ),

    CONSTRAINT provider_event_failures_identity_unique
        UNIQUE (provider, provider_event_id)
);

CREATE INDEX provider_event_failures_status_idx
    ON provider_event_failures (status);

CREATE INDEX provider_event_failures_payment_idx
    ON provider_event_failures (payment_id);

CREATE INDEX provider_event_failures_failed_at_idx
    ON provider_event_failures (last_failed_at);

CREATE INDEX provider_event_failures_event_idx
    ON provider_event_failures (event_id);

INSERT INTO schema_migrations (version)
VALUES (10);

COMMIT;
