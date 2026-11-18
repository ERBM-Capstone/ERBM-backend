CREATE TABLE expenses (
    id BIGSERIAL PRIMARY KEY,
    advance_id BIGINT NOT NULL REFERENCES advances(id) ON DELETE RESTRICT,
    expense_date DATE,
    description TEXT,
    merchant_name VARCHAR(255),
    invoice_no VARCHAR(100),
    amount NUMERIC(15,2) CHECK (amount >= 0),
    status VARCHAR(20) NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','CONFIRMED')),
    line_items JSONB NOT NULL DEFAULT '[]',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    confirmed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    confirmed_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_expenses_id_advance UNIQUE (id, advance_id),
    CONSTRAINT chk_expenses_draft_items CHECK (jsonb_typeof(line_items) = 'array'),
    CONSTRAINT chk_expenses_confirmation CHECK (
        (status = 'DRAFT' AND confirmed_by IS NULL AND confirmed_at IS NULL)
        OR (status = 'CONFIRMED' AND amount IS NOT NULL AND confirmed_by IS NOT NULL AND confirmed_at IS NOT NULL)
    )
);
CREATE INDEX idx_expenses_advance_date ON expenses(advance_id, expense_date, id);

CREATE TABLE expense_audit_events (
    id BIGSERIAL PRIMARY KEY,
    advance_id BIGINT NOT NULL REFERENCES advances(id) ON DELETE RESTRICT,
    expense_id BIGINT REFERENCES expenses(id) ON DELETE SET NULL,
    recorded_expense_id BIGINT NOT NULL,
    actor_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action VARCHAR(50) NOT NULL,
    before_data JSONB,
    after_data JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_expense_events_history ON expense_audit_events(advance_id, recorded_expense_id, occurred_at, id);
CREATE INDEX idx_expense_events_expense ON expense_audit_events(expense_id);

CREATE TABLE expense_documents (
    id BIGSERIAL PRIMARY KEY,
    advance_id BIGINT NOT NULL REFERENCES advances(id) ON DELETE RESTRICT,
    expense_id BIGINT NOT NULL,
    storage_provider VARCHAR(20) NOT NULL CHECK (storage_provider IN ('local','gcs')),
    storage_key TEXT NOT NULL,
    storage_generation TEXT,
    original_filename TEXT NOT NULL,
    content_type VARCHAR(100) NOT NULL CHECK (content_type IN ('image/jpeg','image/png','application/pdf')),
    size_bytes BIGINT NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 10000000),
    page_count SMALLINT NOT NULL CHECK (page_count BETWEEN 1 AND 5),
    sha256 CHAR(64) NOT NULL,
    uploaded_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_document_expense_advance FOREIGN KEY (expense_id, advance_id)
        REFERENCES expenses(id, advance_id) ON DELETE CASCADE,
    CONSTRAINT uq_document_storage UNIQUE (storage_provider, storage_key)
);
CREATE INDEX idx_expense_documents_advance ON expense_documents(advance_id, uploaded_at, id);
CREATE INDEX idx_expense_documents_expense ON expense_documents(expense_id);

-- Independent of deleted domain rows, so file deletion survives a DB commit,
-- storage outage or application restart. Only technical metadata is retained.
CREATE TABLE file_deletion_tasks (
    id BIGSERIAL PRIMARY KEY,
    storage_provider VARCHAR(20) NOT NULL CHECK (storage_provider IN ('local','gcs')),
    storage_key TEXT NOT NULL,
    storage_generation TEXT,
    status VARCHAR(30) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PROCESSING','SUCCEEDED')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error_code VARCHAR(100),
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);
CREATE INDEX idx_file_deletion_pending ON file_deletion_tasks(status, next_attempt_at, id);
