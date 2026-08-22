BEGIN;

ALTER TABLE audit_log
    DROP CONSTRAINT IF EXISTS audit_log_actor_user_id_fkey;

ALTER TABLE audit_log
    ADD CONSTRAINT audit_log_actor_user_id_fkey
    FOREIGN KEY (actor_user_id)
    REFERENCES users(id)
    ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION enforce_audit_integrity()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    actor_organization UUID;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION
            'audit_log is append-only: UPDATE is not permitted';
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION
            'audit_log is append-only: DELETE is not permitted';
    END IF;

    IF NEW.actor_type = 'system' THEN
        IF NEW.actor_user_id IS NOT NULL THEN
            RAISE EXCEPTION
                'system audit events cannot have an actor_user_id';
        END IF;
    ELSIF NEW.actor_type = 'user' THEN
        IF NEW.actor_user_id IS NULL THEN
            RAISE EXCEPTION
                'user audit events require an actor_user_id';
        END IF;

        SELECT u.organization_id
        INTO actor_organization
        FROM users u
        WHERE u.id = NEW.actor_user_id;

        IF actor_organization IS NULL THEN
            RAISE EXCEPTION
                'audit actor user % does not exist',
                NEW.actor_user_id;
        END IF;

        IF NEW.organization_id IS NULL
           OR actor_organization <> NEW.organization_id THEN
            RAISE EXCEPTION
                'audit actor organization does not match audit organization';
        END IF;
    ELSE
        RAISE EXCEPTION
            'invalid audit actor type %',
            NEW.actor_type;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS audit_log_integrity_trigger
ON audit_log;

CREATE TRIGGER audit_log_integrity_trigger
BEFORE INSERT OR UPDATE OR DELETE ON audit_log
FOR EACH ROW
EXECUTE FUNCTION enforce_audit_integrity();

INSERT INTO schema_migrations (version)
VALUES (9);

COMMIT;
