BEGIN;

DELETE FROM schema_migrations
WHERE version = 3;

DROP EXTENSION IF EXISTS pgcrypto;

COMMIT;
