BEGIN;

-- Every existing organization must have an owner if it has users.
-- Pick the oldest active user when no owner currently exists.
DO $$
DECLARE
    org RECORD;
    owner_role UUID;
    first_user UUID;
BEGIN
    FOR org IN
        SELECT id
        FROM organizations
    LOOP
        SELECT id INTO owner_role
        FROM roles
        WHERE organization_id = org.id
          AND name = 'owner';

        IF owner_role IS NULL THEN
            CONTINUE;
        END IF;

        IF EXISTS (
            SELECT 1
            FROM user_roles ur
            JOIN roles r ON r.id = ur.role_id
            WHERE r.organization_id = org.id
              AND r.name = 'owner'
        ) THEN
            CONTINUE;
        END IF;

        SELECT u.id
        INTO first_user
        FROM users u
        WHERE u.organization_id = org.id
          AND u.status = 'active'
        ORDER BY u.created_at ASC, u.id ASC
        LIMIT 1;

        IF first_user IS NOT NULL THEN
            INSERT INTO user_roles (user_id, role_id)
            VALUES (first_user, owner_role)
            ON CONFLICT DO NOTHING;
        END IF;
    END LOOP;
END;
$$;

-- A later user becomes owner only when the organization has no
-- assigned role at all. This also handles organizations created
-- before RBAC provisioning was introduced.
CREATE OR REPLACE FUNCTION assign_first_user_owner()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    owner_role UUID;
    has_any_role BOOLEAN;
BEGIN
    PERFORM pg_advisory_xact_lock(
        hashtextextended(NEW.organization_id::text, 0)
    );

    SELECT id
    INTO owner_role
    FROM roles
    WHERE organization_id = NEW.organization_id
      AND name = 'owner';

    IF owner_role IS NULL THEN
        RAISE EXCEPTION
            'owner role is not provisioned for organization %',
            NEW.organization_id;
    END IF;

    SELECT EXISTS (
        SELECT 1
        FROM user_roles ur
        JOIN roles r ON r.id = ur.role_id
        WHERE r.organization_id = NEW.organization_id
    )
    INTO has_any_role;

    IF NOT has_any_role THEN
        INSERT INTO user_roles (user_id, role_id)
        VALUES (NEW.id, owner_role)
        ON CONFLICT DO NOTHING;
    END IF;

    RETURN NEW;
END;
$$;

INSERT INTO schema_migrations (version)
VALUES (8);

COMMIT;
