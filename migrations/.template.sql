-- migrate:up

-- Add new migrations here.
-- Template:
--
-- CREATE TABLE your_table (
--     id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
--     ...
--     created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
--     created_by TEXT,
--     updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
--     updated_by TEXT,
--     deleted_at TIMESTAMPTZ,
--     deleted_by TEXT
-- );

-- migrate:down

-- DROP TABLE IF EXISTS your_table;
