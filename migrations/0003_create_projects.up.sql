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
CREATE INDEX idx_projects_status ON projects (status);

COMMENT ON COLUMN projects.budget IS 'reference only, funds may exceed it';
COMMENT ON COLUMN projects.status IS 'ACTIVE, CLOSED (closing is permanent)';

-- Create project_coordinators table (a project can have many PCs)
CREATE TABLE project_coordinators (
    project_id  BIGINT      NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     BIGINT      NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_by  BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_coordinators_user_id ON project_coordinators (user_id);
