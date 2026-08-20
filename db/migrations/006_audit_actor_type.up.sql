BEGIN;

ALTER TABLE audit_log
    ADD COLUMN actor_type TEXT;

UPDATE audit_log
SET actor_type =
    CASE
        WHEN actor_user_id IS NULL THEN 'system'
        ELSE 'user'
    END
WHERE actor_type IS NULL;

ALTER TABLE audit_log
    ALTER COLUMN actor_type SET DEFAULT 'system';

ALTER TABLE audit_log
    ALTER COLUMN actor_type SET NOT NULL;

ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_actor_type_check
    CHECK (actor_type IN ('user', 'system'));

INSERT INTO schema_migrations (version)
VALUES (6);

COMMIT;
