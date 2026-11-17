-- Accounting confirms one full payment per Advance. The service copies the
-- exact approved amount and writes payment, activation and audit atomically.
CREATE TABLE advance_disbursements (
    id           BIGSERIAL     PRIMARY KEY,
    advance_id   BIGINT        NOT NULL REFERENCES advances(id) ON DELETE RESTRICT,
    amount       NUMERIC(15,2) NOT NULL CHECK (amount > 0),
    disbursed_at TIMESTAMPTZ   NOT NULL,
    confirmed_by BIGINT        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_advance_disbursements_advance UNIQUE (advance_id)
);

COMMENT ON TABLE advance_disbursements IS
    'Single full payment confirmed by Accounting; amount comes from the locked approved Advance';
COMMENT ON COLUMN advance_disbursements.disbursed_at IS
    'Server timestamp when Accounting confirms that full payment has been made';
