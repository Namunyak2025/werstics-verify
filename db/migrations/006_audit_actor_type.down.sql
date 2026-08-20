BEGIN;

ALTER TABLE audit_log
    DROP CONSTRAINT IF EXISTS audit_log_actor_type_check;

ALTER TABLE audit_log
    DROP COLUMN IF EXISTS actor_type;

DELETE FROM schema_migrations
WHERE version = 6;

COMMIT;
