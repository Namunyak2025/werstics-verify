BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

INSERT INTO schema_migrations (version)
VALUES (3);

COMMIT;
