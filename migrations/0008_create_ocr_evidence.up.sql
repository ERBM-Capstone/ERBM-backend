CREATE TABLE ocr_extractions (
    id BIGSERIAL PRIMARY KEY,
    document_id BIGINT NOT NULL REFERENCES expense_documents(id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    model_version VARCHAR(100),
    status VARCHAR(30) NOT NULL CHECK (status IN ('PROCESSING','SUCCEEDED','FAILED')),
    input_sha256 CHAR(64) NOT NULL,
    applied_to_draft BOOLEAN NOT NULL DEFAULT false,
    raw_result JSONB,
    extracted_amount NUMERIC(15,2),
    extracted_currency CHAR(3),
    error_code VARCHAR(100),
    requested_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ocr_extractions_document_created ON ocr_extractions(document_id, created_at, id);
CREATE UNIQUE INDEX uq_ocr_document_processing ON ocr_extractions(document_id) WHERE status = 'PROCESSING';

CREATE TABLE ocr_line_items (
    id BIGSERIAL PRIMARY KEY,
    extraction_id BIGINT NOT NULL REFERENCES ocr_extractions(id) ON DELETE CASCADE,
    line_no INTEGER NOT NULL CHECK (line_no > 0),
    page_no SMALLINT NOT NULL CHECK (page_no BETWEEN 1 AND 5),
    description TEXT,
    quantity NUMERIC(18,6),
    unit_price NUMERIC(15,2),
    line_total NUMERIC(15,2),
    CONSTRAINT uq_ocr_line UNIQUE (extraction_id, line_no)
);
