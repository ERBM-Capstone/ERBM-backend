-- Advance Request and Advance share one record throughout the workflow.
-- Money has no implicit zero or currency default. Unresolved positivity,
-- budget, fiscal-year, cancellation and closing rules are not constrained here.
CREATE TABLE advances (
    id              BIGSERIAL     PRIMARY KEY,
    project_id      BIGINT        NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    purpose         TEXT          NOT NULL,
    amount          NUMERIC(15,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    status          VARCHAR(40)   NOT NULL DEFAULT 'DRAFT',

    submitted_at    TIMESTAMPTZ,
    submitted_by    BIGINT        REFERENCES users(id) ON DELETE RESTRICT,
    approved_at     TIMESTAMPTZ,
    approved_by     BIGINT        REFERENCES users(id) ON DELETE RESTRICT,
    activated_at    TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    closed_by       BIGINT        REFERENCES users(id) ON DELETE RESTRICT,

    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by      BIGINT        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_by      BIGINT        NOT NULL REFERENCES users(id) ON DELETE RESTRICT
);

-- Include activation protection in the same migration as the table.
-- PostgreSQL enforces this even when concurrent requests activate two records.
CREATE UNIQUE INDEX uq_advances_one_active_per_project
    ON advances (project_id)
    WHERE status = 'ACTIVE';

CREATE INDEX idx_advances_project_created
    ON advances (project_id, created_at, id);

COMMENT ON COLUMN advances.amount IS
    'authoritative Advance amount; relation to actual disbursement is decided in the disbursement workflow';
COMMENT ON COLUMN advances.status IS
    'DRAFT, PENDING_PM_APPROVAL, RETURNED_FOR_REVISION, APPROVED, ACTIVE, CLOSED; closing states/transitions require confirmed service rules';

-- Services write an event in the same transaction as the corresponding change.
-- The future repository/service workflow must preserve append-only history.
CREATE TABLE advance_workflow_events (
    id                  BIGSERIAL    PRIMARY KEY,
    advance_id          BIGINT       NOT NULL REFERENCES advances(id) ON DELETE RESTRICT,
    actor_user_id       BIGINT       NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action              VARCHAR(50)  NOT NULL,
    previous_status     VARCHAR(40),
    resulting_status    VARCHAR(40)  NOT NULL,
    reason              TEXT,
    before_data         JSONB,
    after_data          JSONB        NOT NULL,
    occurred_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_advance_events_history
    ON advance_workflow_events (advance_id, occurred_at, id);

COMMENT ON COLUMN advance_workflow_events.after_data IS
    'audit snapshot; monetary values are decimal strings, not credentials or OCR financial totals';
