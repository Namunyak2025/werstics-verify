BEGIN;

CREATE TABLE permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT permissions_name_unique
        UNIQUE (name)
);

CREATE TABLE role_permissions (
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id UUID NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (role_id, permission_id)
);

CREATE INDEX role_permissions_permission_idx
    ON role_permissions (permission_id);

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT auth_sessions_token_hash_unique
        UNIQUE (token_hash)
);

CREATE INDEX auth_sessions_user_idx
    ON auth_sessions (user_id);

CREATE INDEX auth_sessions_expiry_idx
    ON auth_sessions (expires_at);

CREATE INDEX auth_sessions_active_idx
    ON auth_sessions (user_id, expires_at)
    WHERE revoked_at IS NULL;

ALTER TABLE users
    ADD COLUMN last_login_at TIMESTAMPTZ;

INSERT INTO permissions (name, description)
VALUES
    ('organization:read', 'View organization information'),
    ('organization:manage', 'Manage organization settings'),
    ('payment:create', 'Create payment requests'),
    ('payment:read', 'View payments'),
    ('payment:verify', 'Process and verify payment events'),
    ('user:read', 'View organization users'),
    ('user:manage', 'Manage organization users'),
    ('role:read', 'View roles and permissions'),
    ('role:manage', 'Manage roles and permissions'),
    ('audit:read', 'View security and operational audit records')
ON CONFLICT (name) DO NOTHING;

INSERT INTO schema_migrations (version)
VALUES (4);

COMMIT;
