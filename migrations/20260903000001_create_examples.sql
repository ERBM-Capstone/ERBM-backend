-- migrate:up

CREATE TYPE example_status AS ENUM ('active', 'inactive');

CREATE TABLE examples (
    id          UUID            PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255)    NOT NULL,
    description TEXT,
    status      example_status  NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    created_by  TEXT,
    updated_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    updated_by  TEXT,
    deleted_at  TIMESTAMPTZ,
    deleted_by  TEXT
);

CREATE INDEX idx_examples_deleted_at ON examples (deleted_at);
CREATE INDEX idx_examples_updated_at ON examples (updated_at DESC);

-- migrate:down

DROP TABLE IF EXISTS examples;
DROP TYPE  IF EXISTS example_status;
