BEGIN;

DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS payment_verifications;
DROP TABLE IF EXISTS payment_events;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS provider_accounts;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
DROP TABLE IF EXISTS schema_migrations;

COMMIT;
