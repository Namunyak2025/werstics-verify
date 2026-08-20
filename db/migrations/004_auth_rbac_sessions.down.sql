BEGIN;

ALTER TABLE users
    DROP COLUMN IF EXISTS last_login_at;

DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS auth_sessions;
DROP TABLE IF EXISTS permissions;

DELETE FROM schema_migrations
WHERE version = 4;

COMMIT;
