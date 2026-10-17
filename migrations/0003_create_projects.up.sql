-- Create projects table
CREATE TABLE projects (
    id              BIGSERIAL     PRIMARY KEY,
    project_code    VARCHAR(50)   NOT NULL,
    name            VARCHAR(255)  NOT NULL,
    description     TEXT,
    budget          NUMERIC(15,2) NOT NULL CHECK (budget > 0),
    start_date      DATE,
    end_date        DATE,

    status          VARCHAR(20)   NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CLOSED')),
    pm_user_id      BIGINT        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    pc_user_id      BIGINT        REFERENCES users(id) ON DELETE RESTRICT,

    closed_at       TIMESTAMPTZ,
    closed_by       BIGINT        REFERENCES users(id) ON DELETE SET NULL,

    created_by      BIGINT        REFERENCES users(id) ON DELETE SET NULL,
    updated_by      BIGINT        REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_projects_dates CHECK (end_date IS NULL OR start_date IS NULL OR end_date >= start_date)
);

CREATE UNIQUE INDEX uq_projects_project_code ON projects (UPPER(project_code));
CREATE INDEX idx_projects_pm_user_id ON projects (pm_user_id);
CREATE INDEX idx_projects_pc_user_id ON projects (pc_user_id);
CREATE INDEX idx_projects_status ON projects (status);

-- Allocation and Project insertion share a transaction. Each Gregorian year
-- in Asia/Bangkok has an independent sequence starting at 1.
CREATE TABLE project_code_counters (
    year        INTEGER PRIMARY KEY,
    last_number BIGINT NOT NULL CHECK (last_number > 0)
);

COMMENT ON COLUMN projects.project_code IS
    'Immutable server-generated code: PRJ-{Gregorian year in Asia/Bangkok}-{number, minimum 6 digits}';

COMMENT ON COLUMN projects.budget IS 'reference only, funds may exceed it';
COMMENT ON COLUMN projects.status IS 'ACTIVE, CLOSED (closing is permanent)';

COMMENT ON COLUMN projects.pc_user_id IS
    'Optional assigned PC; a PC may coordinate multiple projects';
