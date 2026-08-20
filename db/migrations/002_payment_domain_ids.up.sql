BEGIN;

ALTER TABLE payments
    ADD COLUMN payment_id TEXT;

UPDATE payments
SET payment_id = id::text
WHERE payment_id IS NULL;

ALTER TABLE payments
    ALTER COLUMN payment_id SET NOT NULL;

ALTER TABLE payments
    ADD CONSTRAINT payments_payment_id_unique
    UNIQUE (payment_id);

CREATE INDEX payments_payment_id_idx
    ON payments (payment_id);

COMMIT;
