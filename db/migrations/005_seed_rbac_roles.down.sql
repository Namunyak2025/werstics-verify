BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id
    FROM roles
    WHERE organization_id = '11111111-1111-4111-8111-111111111111'::uuid
      AND name IN ('owner', 'admin', 'operator', 'viewer')
);

DELETE FROM roles
WHERE organization_id = '11111111-1111-4111-8111-111111111111'::uuid
  AND name IN ('owner', 'admin', 'operator', 'viewer');

DELETE FROM schema_migrations
WHERE version = 5;

COMMIT;
