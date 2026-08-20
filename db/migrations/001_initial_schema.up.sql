BEGIN;

CREATE TABLE organizations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT organizations_status_check
        CHECK (status IN ('active', 'suspended', 'disabled'))
);

CREATE TABLE users (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT users_status_check
        CHECK (status IN ('active', 'suspended', 'disabled')),

    CONSTRAINT users_organization_email_unique
        UNIQUE (organization_id, email)
);

CREATE INDEX users_organization_id_idx
    ON users (organization_id);

CREATE TABLE roles (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT roles_organization_name_unique
        UNIQUE (organization_id, name)
);

CREATE TABLE user_roles (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE provider_accounts (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    external_account_ref TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT provider_accounts_status_check
        CHECK (status IN ('active', 'disabled')),

    CONSTRAINT provider_accounts_external_unique
        UNIQUE (provider, external_account_ref)
);

CREATE INDEX provider_accounts_organization_idx
    ON provider_accounts (organization_id);

CREATE TABLE payments (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id),
    merchant_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    provider_account_id UUID REFERENCES provider_accounts(id),
    provider TEXT NOT NULL,
    provider_ref TEXT,
    expected_currency CHAR(3) NOT NULL,
    expected_minor BIGINT NOT NULL,
    received_currency CHAR(3),
    received_minor BIGINT,
    customer_display TEXT,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT payments_expected_amount_positive
        CHECK (expected_minor > 0),

    CONSTRAINT payments_received_amount_positive
        CHECK (received_minor IS NULL OR received_minor > 0)
);

CREATE INDEX payments_organization_idx
    ON payments (organization_id);

CREATE INDEX payments_merchant_idx
    ON payments (merchant_id);

CREATE INDEX payments_session_idx
    ON payments (session_id);

CREATE INDEX payments_provider_ref_idx
    ON payments (provider, provider_ref);

CREATE INDEX payments_status_idx
    ON payments (status);

CREATE TABLE payment_events (
    id UUID PRIMARY KEY,
    payment_id UUID REFERENCES payments(id),
    event_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_event_id TEXT NOT NULL,
    provider_ref TEXT,
    merchant_id TEXT NOT NULL,
    amount_currency CHAR(3) NOT NULL,
    amount_minor BIGINT NOT NULL,
    customer_display TEXT,
    kind TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processing_status TEXT NOT NULL DEFAULT 'received',

    CONSTRAINT payment_events_amount_positive
        CHECK (amount_minor > 0),

    CONSTRAINT payment_events_event_id_unique
        UNIQUE (event_id),

    CONSTRAINT payment_events_provider_event_unique
        UNIQUE (provider, provider_event_id)
);

CREATE INDEX payment_events_payment_idx
    ON payment_events (payment_id);

CREATE INDEX payment_events_provider_ref_idx
    ON payment_events (provider, provider_ref);

CREATE INDEX payment_events_occurred_at_idx
    ON payment_events (occurred_at);

CREATE TABLE payment_verifications (
    id UUID PRIMARY KEY,
    payment_id UUID NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    event_id UUID NOT NULL REFERENCES payment_events(id),
    matched BOOLEAN NOT NULL,
    amount_matched BOOLEAN NOT NULL,
    merchant_matched BOOLEAN NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT payment_verifications_event_unique
        UNIQUE (event_id)
);

CREATE INDEX payment_verifications_payment_idx
    ON payment_verifications (payment_id);

CREATE TABLE audit_log (
    id UUID PRIMARY KEY,
    organization_id UUID REFERENCES organizations(id),
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_log_organization_idx
    ON audit_log (organization_id);

CREATE INDEX audit_log_actor_idx
    ON audit_log (actor_user_id);

CREATE INDEX audit_log_resource_idx
    ON audit_log (resource_type, resource_id);

CREATE INDEX audit_log_created_at_idx
    ON audit_log (created_at);

CREATE TABLE schema_migrations (
    version BIGINT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_migrations (version)
VALUES (1);

COMMIT;
