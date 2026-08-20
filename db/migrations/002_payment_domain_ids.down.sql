BEGIN;

DROP INDEX IF EXISTS payments_payment_id_idx;

ALTER TABLE payments
    DROP CONSTRAINT IF EXISTS payments_payment_id_unique;

ALTER TABLE payments
    DROP COLUMN IF EXISTS payment_id;

COMMIT;
