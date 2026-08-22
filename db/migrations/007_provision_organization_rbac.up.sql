BEGIN;

CREATE OR REPLACE FUNCTION provision_organization_rbac(
    p_organization_id UUID
)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
    owner_role UUID;
    admin_role UUID;
    operator_role UUID;
    viewer_role UUID;
BEGIN
    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), p_organization_id, 'owner')
    ON CONFLICT (organization_id, name) DO NOTHING;

    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), p_organization_id, 'admin')
    ON CONFLICT (organization_id, name) DO NOTHING;

    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), p_organization_id, 'operator')
    ON CONFLICT (organization_id, name) DO NOTHING;

    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), p_organization_id, 'viewer')
    ON CONFLICT (organization_id, name) DO NOTHING;

    SELECT id INTO owner_role
    FROM roles
    WHERE organization_id = p_organization_id
      AND name = 'owner';

    SELECT id INTO admin_role
    FROM roles
    WHERE organization_id = p_organization_id
      AND name = 'admin';

    SELECT id INTO operator_role
    FROM roles
    WHERE organization_id = p_organization_id
      AND name = 'operator';

    SELECT id INTO viewer_role
    FROM roles
    WHERE organization_id = p_organization_id
      AND name = 'viewer';

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT owner_role, id
    FROM permissions
    ON CONFLICT DO NOTHING;

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT admin_role, id
    FROM permissions
    WHERE name <> 'role:manage'
    ON CONFLICT DO NOTHING;

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT operator_role, id
    FROM permissions
    WHERE name IN (
        'organization:read',
        'payment:create',
        'payment:read',
        'payment:verify'
    )
    ON CONFLICT DO NOTHING;

    INSERT INTO role_permissions (role_id, permission_id)
    SELECT viewer_role, id
    FROM permissions
    WHERE name IN (
        'organization:read',
        'payment:read'
    )
    ON CONFLICT DO NOTHING;
END;
$$;

CREATE OR REPLACE FUNCTION provision_organization_rbac_trigger()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM provision_organization_rbac(NEW.id);
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS organizations_provision_rbac
ON organizations;

CREATE TRIGGER organizations_provision_rbac
AFTER INSERT ON organizations
FOR EACH ROW
EXECUTE FUNCTION provision_organization_rbac_trigger();

-- Provision RBAC for organizations that already existed before this migration.
DO $$
DECLARE
    organization_record RECORD;
BEGIN
    FOR organization_record IN
        SELECT id
        FROM organizations
    LOOP
        PERFORM provision_organization_rbac(
            organization_record.id
        );
    END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION assign_first_user_owner()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    owner_role UUID;
BEGIN
    -- Serialize first-user provisioning per organization.
    PERFORM pg_advisory_xact_lock(
        hashtextextended(NEW.organization_id::text, 0)
    );

    SELECT id INTO owner_role
    FROM roles
    WHERE organization_id = NEW.organization_id
      AND name = 'owner';

    IF owner_role IS NULL THEN
        RAISE EXCEPTION
            'owner role is not provisioned for organization %',
            NEW.organization_id;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM users
        WHERE organization_id = NEW.organization_id
          AND id <> NEW.id
    ) THEN
        INSERT INTO user_roles (user_id, role_id)
        VALUES (NEW.id, owner_role)
        ON CONFLICT DO NOTHING;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS users_assign_first_owner
ON users;

CREATE TRIGGER users_assign_first_owner
AFTER INSERT ON users
FOR EACH ROW
EXECUTE FUNCTION assign_first_user_owner();

INSERT INTO schema_migrations (version)
VALUES (7);

COMMIT;
