BEGIN;

DO $$
DECLARE
    org_id UUID := '11111111-1111-4111-8111-111111111111';
    owner_role UUID;
    admin_role UUID;
    operator_role UUID;
    viewer_role UUID;
BEGIN
    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), org_id, 'owner')
    ON CONFLICT (organization_id, name)
    DO UPDATE SET name = EXCLUDED.name
    RETURNING id INTO owner_role;

    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), org_id, 'admin')
    ON CONFLICT (organization_id, name)
    DO UPDATE SET name = EXCLUDED.name
    RETURNING id INTO admin_role;

    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), org_id, 'operator')
    ON CONFLICT (organization_id, name)
    DO UPDATE SET name = EXCLUDED.name
    RETURNING id INTO operator_role;

    INSERT INTO roles (id, organization_id, name)
    VALUES (gen_random_uuid(), org_id, 'viewer')
    ON CONFLICT (organization_id, name)
    DO UPDATE SET name = EXCLUDED.name
    RETURNING id INTO viewer_role;

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
END $$;

INSERT INTO schema_migrations (version)
VALUES (5);

COMMIT;
